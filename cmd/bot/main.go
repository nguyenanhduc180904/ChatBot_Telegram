package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go-finance/internal/model"
	"go-finance/internal/service"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

var apiURL string

func main() {
	// 1. >>> GIỮ KẾT NỐI VỚI RENDER <<<
	// Chạy một HTTP server giả trên cổng 8080 (hoặc cổng Render cung cấp)
	go func() {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		log.Printf("Listening on port %s to satisfy Render health check...", port)
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("Bot is running!"))
		})
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatal(err)
		}
	}()
	// ----------------------------------------------------

	_ = godotenv.Load()
	token := os.Getenv("TELEGRAM_TOKEN")
	apiURL = os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}
	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Xóa webhook cũ để chuyển sang chế độ Polling
	_, err = bot.Request(tgbotapi.DeleteWebhookConfig{})
	if err != nil {
		log.Printf("Lỗi xóa webhook: %v", err)
	}
	// -------------------------

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		text := update.Message.Text
		userID := fmt.Sprintf("%d", update.Message.From.ID)

		// 1. Lệnh Báo cáo
		if strings.Contains(strings.ToLower(text), "báo cáo") {
			handleReport(bot, update.Message.Chat.ID, userID)
			continue
		}

		// 2. Lệnh Giá cả (TÁCH RIÊNG VÀNG VÀ BẠC)
		if strings.Contains(strings.ToLower(text), "giá vàng") {
			handlePrice(bot, update.Message.Chat.ID, "gold")
			continue
		}
		if strings.Contains(strings.ToLower(text), "giá bạc") {
			handlePrice(bot, update.Message.Chat.ID, "silver")
			continue
		}

		// 3. Xử lý nhập liệu
		txs, _ := service.ParseTransactionText(text)

		// CẬP NHẬT: Thay đổi thông báo khi không hiểu lệnh
		if len(txs) == 0 {
			helpMsg := `Không hiểu lệnh. Vui lòng nhập đúng cú pháp.
			👋 Chào bạn! Tôi là Bot quản lý tài chính.

			📖 *HƯỚNG DẪN SỬ DỤNG:*

			1️⃣ *Ghi chép Thu / Chi (VND):*
			_(Bắt buộc phải kèm lý do)_
			- chi 50k ăn trưa
			- thu 10m lương t10
			- -10k trà đá
			- +1,5m tiền lãi bank

			2️⃣ *Ghi chép Tiết kiệm / Đầu tư:*
			_(Chỉ nhập số tiền & đơn vị, KHÔNG ghi chú)_
			- tk 2m
			- tiết kiệm 100 usd
			- tk 0.1 btc
			- tk 5 chỉ vàng

			3️⃣ *Tiện ích khác:*
			- giá vàng, giá bạc
			- báo cáo`

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, helpMsg)
			msg.ParseMode = "Markdown" // Kích hoạt in đậm
			bot.Send(msg)
			continue
		}

		// Gọi API để lưu từng transaction
		count := 0
		var details []string
		for _, tx := range txs {
			tx.UserID = userID
			if sendTransactionToAPI(tx) {
				count++
				details = append(details, fmt.Sprintf("%s %.2f %s", tx.Type, tx.Amount, tx.Currency))
			}
		}

		reply := fmt.Sprintf("Đã lưu %d giao dịch:\n%s", count, strings.Join(details, "\n"))
		bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, reply))
	}
}

// --- LOGIC THU, CHI, TIẾT KIỆM ---
// Hàm gửi transaction lên API
func sendTransactionToAPI(t model.TransactionCreate) bool {
	data, _ := json.Marshal(t)
	// In ra log để debug URL
	log.Printf("Đang gọi API: %s/transactions", apiURL)

	resp, err := http.Post(apiURL+"/transactions", "application/json", bytes.NewBuffer(data))
	if err != nil {
		// [QUAN TRỌNG] In lỗi mạng (ví dụ: connection refused, timeout...)
		log.Printf("❌ Lỗi kết nối API: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// [QUAN TRỌNG] In lỗi từ Server (ví dụ: 404, 500...)
		body, _ := io.ReadAll(resp.Body)
		log.Printf("❌ API trả về lỗi: Code %d - Body: %s", resp.StatusCode, string(body))
		return false
	}

	return true
}

// --- LOGIC BÁO CÁO ---
// bot trả về báo cáo tuần/tháng
func handleReport(bot *tgbotapi.BotAPI, chatID int64, userID string) {
	// 1. Lấy dữ liệu TUẦN
	weekReport, err := getReportData(userID, "week")
	if err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, "Lỗi lấy báo cáo tuần"))
		return
	}

	// 2. Lấy dữ liệu THÁNG
	monthReport, err := getReportData(userID, "month")
	if err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, "Lỗi lấy báo cáo tháng"))
		return
	}

	// 3. Ghép nội dung
	finalMsg := "📊 BÁO CÁO TÀI CHÍNH\n\n"
	finalMsg += buildSectionReport("Tuần này", weekReport)
	finalMsg += "\n" + strings.Repeat("-", 20) + "\n\n" // Đường kẻ ngang phân cách
	finalMsg += buildSectionReport("Tháng này", monthReport)

	msg := tgbotapi.NewMessage(chatID, finalMsg)
	bot.Send(msg)
}

// Hàm gọi API lấy báo cáo
func getReportData(userID string, period string) (*model.ReportOutput, error) {
	url := fmt.Sprintf("%s/report?user_id=%s&period=%s", apiURL, userID, period)
	log.Printf("Đang lấy báo cáo từ: %s", url) // Log URL

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("❌ Lỗi mạng khi lấy báo cáo: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("❌ API báo cáo lỗi: Code %d - %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API error: %d", resp.StatusCode)
	}

	var r model.ReportOutput
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Hàm build string cho một phần báo cáo (Tuần hoặc Tháng)
func buildSectionReport(title string, r *model.ReportOutput) string {
	// Thu - Chi - Dư
	text := fmt.Sprintf("📅 *%s:*\n", title)
	text += fmt.Sprintf("   📈 Thu: %s đ\n", formatCurrency(r.TotalIncome))
	text += fmt.Sprintf("   📉 Chi: %s đ\n", formatCurrency(r.TotalExpense))
	text += fmt.Sprintf("   🐷 Đã nạp tiết kiệm: %s đ\n", formatCurrency(r.TotalSavingsVND))
	text += fmt.Sprintf("   👉 Dư(Thu - Chi tiêu - Tiền đem đi cất): %s đ\n", formatCurrency(r.Balance))

	// Chi theo nhóm
	if len(r.ExpenseByCategory) > 0 {
		text += "   - Chi theo nhóm:\n"
		for cat, val := range r.ExpenseByCategory {
			// Viết hoa chữ cái đầu category cho đẹp
			catName := strings.Title(cat)
			text += fmt.Sprintf("     + %s: %s đ\n", catName, formatCurrency(val))
		}
	}

	// Tài sản tích lũy
	text += fmt.Sprintf("   💰 Tài sản tích lũy theo %s:\n", strings.ToLower(title))
	hasAsset := false
	for currency, asset := range r.Assets {
		if asset.Quantity > 0 {
			hasAsset = true
			// Format: - 4,010 USD (Tỷ giá: 26,229) = 105,176,294 đ
			text += fmt.Sprintf("     - %s %s (Tỷ giá: %s) = %s đ\n",
				formatAssetQty(asset.Quantity),
				currency,
				formatCurrency(asset.Rate),
				formatCurrency(asset.CurrentVND))
		}
	}
	if !hasAsset {
		text += "     (Chưa có tài sản mới)\n"
	}
	text += fmt.Sprintf("   👉 Tổng trị giá tài sản tích lũy theo %s: %s đ\n", strings.ToLower(title), formatCurrency(r.TotalAssetsVND))

	return text
}

// Hàm định dạng tiền tệ: 1000000 -> 1,000,000
func formatCurrency(amount float64) string {
	// Chuyển sang int để bỏ phần thập phân nếu là số nguyên
	s := fmt.Sprintf("%.0f", amount)
	// Logic thêm dấu phẩy
	if len(s) <= 3 {
		return s
	}
	var result []byte
	count := 0
	for i := len(s) - 1; i >= 0; i-- {
		count++
		result = append([]byte{s[i]}, result...)
		if count%3 == 0 && i > 0 && s[i-1] != '-' {
			result = append([]byte{','}, result...)
		}
	}
	return string(result)
}

// Hàm format riêng cho ngoại tệ (giữ lại số lẻ nếu cần)
func formatAssetQty(qty float64) string {
	// Nếu tròn chẵn thì bỏ .00, nếu lẻ thì giữ tối đa 4 số
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", qty), "0"), ".")
}

// --- LOGIC GIÁ VÀNG BẠC ---
func handlePrice(bot *tgbotapi.BotAPI, chatID int64, requestType string) {
	resp, err := http.Get(apiURL + "/market-rates")
	if err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, "⚠️ Lỗi kết nối lấy giá."))
		return
	}
	defer resp.Body.Close()

	var r model.ExchangeRates
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, "⚠️ Lỗi đọc dữ liệu giá."))
		return
	}

	const OunceToTael = 1.20565
	var msgBuf bytes.Buffer

	// Luôn hiển thị Tỷ giá USD đầu tiên
	msgBuf.WriteString(fmt.Sprintf("🔔 TỶ GIÁ: 1 USD = %s VNĐ\n\n", formatCurrency(r.UsdVND)))

	// SECTION: VÀNG
	if requestType == "gold" {
		convertedGold := r.GoldUSD * r.UsdVND * OunceToTael

		msgBuf.WriteString("🏆 VÀNG (GOLD)\n")
		msgBuf.WriteString(fmt.Sprintf("• Thế giới: %s USD/oz\n", formatUSD(r.GoldUSD)))
		msgBuf.WriteString(fmt.Sprintf("• Quy đổi: %s đ/lượng\n", formatCurrency(convertedGold)))
		msgBuf.WriteString(fmt.Sprintf("• SJC (Thực tế): %s đ/lượng\n", formatCurrency(r.VnSJC)))

		// Chênh lệch Vàng
		diffGold := r.VnSJC - convertedGold
		statusGold := "VN cao hơn"
		if diffGold < 0 {
			statusGold = "VN thấp hơn"
		}
		msgBuf.WriteString(fmt.Sprintf("⚖️ Chênh lệch: %s %s đ", statusGold, formatCurrency(math.Abs(diffGold))))
	}

	// SECTION: BẠC
	if requestType == "silver" {
		convertedSilver := r.SilverUSD * r.UsdVND * OunceToTael

		msgBuf.WriteString("ww BẠC (SILVER)\n")
		msgBuf.WriteString(fmt.Sprintf("• Thế giới: %s USD/oz\n", formatUSD(r.SilverUSD)))
		msgBuf.WriteString(fmt.Sprintf("• Quy đổi: %s đ/lượng\n", formatCurrency(convertedSilver)))
		msgBuf.WriteString(fmt.Sprintf("• VN (Thực tế): %s đ/lượng\n", formatCurrency(r.VnSilver)))

		// Chênh lệch Bạc
		diffSilver := r.VnSilver - convertedSilver
		statusSilver := "VN cao hơn"
		if diffSilver < 0 {
			statusSilver = "VN thấp hơn"
		}
		msgBuf.WriteString(fmt.Sprintf("⚖️ Chênh lệch: %s %s đ", statusSilver, formatCurrency(math.Abs(diffSilver))))
	}

	bot.Send(tgbotapi.NewMessage(chatID, msgBuf.String()))
}

// Helper: Format số USD (ví dụ: 2,645.50)
func formatUSD(amount float64) string {
	// Format 2 số thập phân, ví dụ: 2645.50
	s := fmt.Sprintf("%.2f", amount)

	// Logic thêm dấu phẩy cho phần nguyên (đơn giản hoá)
	parts := strings.Split(s, ".")
	integerPart := parts[0]
	decimalPart := parts[1]

	var result []byte
	count := 0
	for i := len(integerPart) - 1; i >= 0; i-- {
		count++
		result = append([]byte{integerPart[i]}, result...)
		if count%3 == 0 && i > 0 && integerPart[i-1] != '-' {
			result = append([]byte{','}, result...)
		}
	}
	return string(result) + "." + decimalPart
}
