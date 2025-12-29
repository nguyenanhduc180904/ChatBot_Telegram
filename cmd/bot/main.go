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
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

var apiURL string

func main() {
	_ = godotenv.Load()
	token := os.Getenv("TELEGRAM_TOKEN")
	apiURL = os.Getenv("API_URL")
	// Chạy ngầm nhiệm vụ Ping API cứ 10 phút/lần
	go keepAliveService(apiURL, "API-Service")
	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		webhookURL = os.Getenv("RENDER_EXTERNAL_URL")
	}
	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
	}
	if apiURL == "" {
		// [Update] Cảnh báo nếu thiếu API URL
		log.Println("[CONFIG WARN] API_URL is empty, defaulting to localhost (This will fail on Render!)")
		apiURL = "http://localhost:8080"
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}
	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Kênh nhận tin nhắn (Updates Channel)
	var updates tgbotapi.UpdatesChannel

	// --- LOGIC CHUYỂN ĐỔI WEBHOOK / POLLING ---
	if webhookURL != "" {
		// >>> CHẾ ĐỘ WEBHOOK (Chạy trên Render) <<<
		log.Printf("[MODE] Running in WEBHOOK mode. URL: %s", webhookURL)

		// 1. Cấu hình Webhook lên Telegram Server
		// Lưu ý: Telegram yêu cầu đường dẫn phải HTTPS
		wh, _ := tgbotapi.NewWebhook(webhookURL + "/webhook")
		_, err = bot.Request(wh)
		if err != nil {
			log.Fatal("Lỗi thiết lập Webhook:", err)
		}

		// 2. Lấy info webhook để confirm
		info, err := bot.GetWebhookInfo()
		if err != nil {
			log.Fatal(err)
		}
		if info.LastErrorDate != 0 {
			log.Printf("Telegram Webhook Last Error: %s", info.LastErrorMessage)
		}

		// 3. Tạo Handler lắng nghe từ Telegram
		// Đường dẫn này khớp với phần cấu hình NewWebhook ở trên
		updates = bot.ListenForWebhook("/webhook")

		// 4. Khởi chạy HTTP Server
		// Server này vừa nhận Webhook từ Telegram, vừa health check cho Render
		go func() {
			// Route health check đơn giản
			http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("Bot is running in Webhook mode!"))
			})

			// tgbotapi.ListenForWebhook tự động đăng ký handler vào http.DefaultServeMux
			// nên ta chỉ cần start server
			log.Printf("Listening on port %s for Webhook...", port)
			if err := http.ListenAndServe(":"+port, nil); err != nil {
				log.Fatal(err)
			}
		}()

	} else {
		// >>> CHẾ ĐỘ POLLING (Chạy Local) <<<
		log.Printf("[MODE] Running in POLLING mode (No WEBHOOK_URL found)")

		// 1. Xóa Webhook cũ (nếu có) để chuyển về Polling
		_, err = bot.Request(tgbotapi.DeleteWebhookConfig{})
		if err != nil {
			log.Printf("Lỗi xóa webhook: %v", err)
		}

		// 2. Tạo config Polling
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		updates = bot.GetUpdatesChan(u)

		// 3. Vẫn chạy một server ảo để health check (nếu chạy docker local)
		go func() {
			log.Printf("Listening on port %s (Dummy Server for Health Check)...", port)
			http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("Bot is running in Polling mode!"))
			})
			http.ListenAndServe(":"+port, nil)
		}()
	}

	// Bắt đầu chạy lịch trình gửi tin 7h sáng/tối
	go startScheduler(bot)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		go func(update tgbotapi.Update) {
			text := update.Message.Text
			userID := fmt.Sprintf("%d", update.Message.From.ID)
			chatID := update.Message.Chat.ID

			log.Printf("[BOT RECV] User: %s, Text: %s", userID, text) // [Update] Log tin nhắn đến

			if strings.Contains(strings.ToLower(text), "báo cáo") {
				handleReport(bot, chatID, userID)
				return
			}

			if strings.Contains(strings.ToLower(text), "giá vàng") {
				handlePrice(bot, chatID, "gold")
				return
			}
			if strings.Contains(strings.ToLower(text), "giá bạc") {
				handlePrice(bot, chatID, "silver")
				return
			}

			// Test gửi thông báo định kỳ
			if text == "/test_noti" {
				bot.Send(tgbotapi.NewMessage(chatID, "🚀 Đang chạy thử tính năng gửi Noti..."))
				sendDailyUpdate(bot)
				return
			}

			txs, _ := service.ParseTransactionText(text)
			if len(txs) == 0 {
				// (Giữ nguyên phần helpMsg của bạn ở đây...)
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
				bot.Send(tgbotapi.NewMessage(chatID, helpMsg))
				return
			}

			count := 0
			var details []string
			for _, tx := range txs {
				tx.UserID = userID
				if sendTransactionToAPI(tx) {
					count++
					details = append(details, fmt.Sprintf("%s %.2f %s", tx.Type, tx.Amount, tx.Currency))
				} else {
					// [Update] Báo lỗi ngay cho user nếu lưu thất bại
					bot.Send(tgbotapi.NewMessage(chatID, "❌ Lỗi hệ thống: Không thể lưu giao dịch."))
				}
			}

			if count > 0 {
				reply := fmt.Sprintf("✅ Đã lưu %d giao dịch:\n%s", count, strings.Join(details, "\n"))
				bot.Send(tgbotapi.NewMessage(chatID, reply))
			}
		}(update)

	}
}

// --- LOGIC THU, CHI, TIẾT KIỆM ---
func sendTransactionToAPI(t model.TransactionCreate) bool {
	data, _ := json.Marshal(t)
	resp, err := http.Post(apiURL+"/transactions", "application/json", bytes.NewBuffer(data))

	// [Update] Log chi tiết lỗi kết nối
	if err != nil {
		log.Printf("[BOT ERROR] Call API /transactions failed: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[BOT ERROR] API returned status %d: %s", resp.StatusCode, string(body))
		return false
	}
	return true
}

// --- LOGIC BÁO CÁO ---
func handleReport(bot *tgbotapi.BotAPI, chatID int64, userID string) {
	// [Update] Thêm log lỗi vào đây
	weekReport, err := getReportData(userID, "week")
	if err != nil {
		log.Printf("[BOT ERROR] Get week report failed: %v", err)
		bot.Send(tgbotapi.NewMessage(chatID, "❌ Lỗi lấy báo cáo tuần"))
		return
	}

	monthReport, err := getReportData(userID, "month")
	if err != nil {
		log.Printf("[BOT ERROR] Get month report failed: %v", err)
		bot.Send(tgbotapi.NewMessage(chatID, "❌ Lỗi lấy báo cáo tháng"))
		return
	}

	// (Giữ nguyên logic buildSectionReport...)
	finalMsg := "📊 BÁO CÁO TÀI CHÍNH\n\n"
	finalMsg += buildSectionReport("Tuần này", weekReport)
	finalMsg += "\n" + strings.Repeat("-", 20) + "\n\n"
	finalMsg += buildSectionReport("Tháng này", monthReport)

	bot.Send(tgbotapi.NewMessage(chatID, finalMsg))
}

// Hàm gọi API lấy báo cáo
func getReportData(userID string, period string) (*model.ReportOutput, error) {
	url := fmt.Sprintf("%s/report?user_id=%s&period=%s", apiURL, userID, period)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API status %d: %s", resp.StatusCode, string(body))
	}

	var r model.ReportOutput
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("Decode json error: %v", err)
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
		msgBuf.WriteString(fmt.Sprintf("• Quy đổi: %s đ/cây\n", formatCurrency(convertedGold)))
		msgBuf.WriteString(fmt.Sprintf("• SJC (Thực tế): %s đ/cây\n", formatCurrency(r.VnSJC*10)))

		// Chênh lệch Vàng
		statusGold := "VN cao hơn"
		if r.GoldDiff < 0 {
			statusGold = "VN thấp hơn"
		}
		msgBuf.WriteString(fmt.Sprintf("⚖️ Chênh lệch: %s %s đ", statusGold, formatCurrency(math.Abs(r.GoldDiff))))
	}

	// SECTION: BẠC
	if requestType == "silver" {
		convertedSilver := r.SilverUSD * r.UsdVND * OunceToTael

		msgBuf.WriteString("ww BẠC (SILVER)\n")
		msgBuf.WriteString(fmt.Sprintf("• Thế giới: %s USD/oz\n", formatUSD(r.SilverUSD)))
		msgBuf.WriteString(fmt.Sprintf("• Quy đổi: %s đ/cây\n", formatCurrency(convertedSilver)))
		msgBuf.WriteString(fmt.Sprintf("• VN (Thực tế): %s đ/cây\n", formatCurrency(r.VnSilver)))

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

// --- LOGIC SCHEDULER GỬI TIN NHẮN ĐỊNH KỲ ---
func startScheduler(bot *tgbotapi.BotAPI) {
	// Định nghĩa múi giờ Việt Nam (UTC+7)
	loc := time.FixedZone("ICT", 7*3600)

	// Kiểm tra mỗi phút một lần
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for t := range ticker.C {
		// Chuyển về giờ VN
		localTime := t.In(loc)
		hour := localTime.Hour()
		minute := localTime.Minute()

		// Nếu là 7:00 hoặc 19:00 (7h tối)
		if (hour == 7 || hour == 19) && minute == 0 {
			log.Println("[SCHEDULER] Bắt đầu gửi thông báo định kỳ...")
			sendDailyUpdate(bot)
			// Ngủ 65 giây để tránh gửi lặp lại trong cùng 1 phút đó
			time.Sleep(65 * time.Second)
		}
	}
}

func sendDailyUpdate(bot *tgbotapi.BotAPI) {
	// 1. Lấy dữ liệu giá cả
	resp, err := http.Get(apiURL + "/market-rates")
	if err != nil {
		log.Printf("[SCHEDULER ERROR] Không thể lấy giá: %v", err)
		return
	}
	defer resp.Body.Close()

	var r model.ExchangeRates
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		log.Printf("[SCHEDULER ERROR] Lỗi decode giá: %v", err)
		return
	}

	// 2. Soạn nội dung tin nhắn
	const OunceToTael = 1.20565
	goldVND := r.GoldUSD * r.UsdVND * OunceToTael
	silverVND := r.SilverUSD * r.UsdVND * OunceToTael

	msgContent := fmt.Sprintf(
		"🔔 *BẢN TIN THỊ TRƯỜNG (7H)* 🔔\n\n"+
			"🇺🇸 *USD:* %s VNĐ\n"+
			"🏆 *Vàng (TG):* %s VNĐ/cây\n"+
			"   _(Vàng SJC: %s VNĐ/cây)_\n"+
			"ww *Bạc (TG):* %s VNĐ/cây\n"+
			"🅱️ *Bitcoin:* %s VNĐ\n",
		formatCurrency(r.UsdVND),
		formatCurrency(goldVND),
		formatCurrency(r.VnSJC*10),
		formatCurrency(silverVND),
		formatCurrency(r.BtcVND),
	)

	// 3. Lấy danh sách Users
	userResp, err := http.Get(apiURL + "/users")
	if err != nil {
		log.Printf("[SCHEDULER ERROR] Không thể lấy user list: %v", err)
		return
	}
	defer userResp.Body.Close()

	var userIDs []string
	if err := json.NewDecoder(userResp.Body).Decode(&userIDs); err != nil {
		log.Printf("[SCHEDULER ERROR] Lỗi decode user list: %v", err)
		return
	}

	// 4. Gửi tin nhắn cho từng người
	count := 0
	for _, uidStr := range userIDs {
		// Chuyển uid string -> int64
		chatID, err := strconv.ParseInt(uidStr, 10, 64)
		if err != nil {
			continue
		}

		msg := tgbotapi.NewMessage(chatID, msgContent)
		msg.ParseMode = "Markdown"
		if _, err := bot.Send(msg); err == nil {
			count++
		}
	}
	log.Printf("[SCHEDULER] Đã gửi thông báo cho %d người dùng.", count)
}

// --- GIỮ BOT KHÔNG NGỦ ---
func keepAliveService(targetURL string, serviceName string) {
	if targetURL == "" {
		log.Printf("[%s] Không có URL để ping. Bỏ qua.", serviceName)
		return
	}

	// Ping mỗi 10 phút (Render ngủ sau 15 phút)
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	log.Printf("[%s] Đã kích hoạt chế độ Keep-Alive tới: %s", serviceName, targetURL)

	for range ticker.C {
		resp, err := http.Get(targetURL)
		if err != nil {
			log.Printf("[%s] Ping thất bại: %v", serviceName, err)
		} else {
			// Quan trọng: Phải đóng Body để tránh rò rỉ bộ nhớ
			resp.Body.Close()
			log.Printf("[%s] Ping thành công! (Status: %s)", serviceName, resp.Status)
		}
	}
}
