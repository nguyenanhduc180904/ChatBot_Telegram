package handler

import (
	"encoding/json"
	"go-finance/internal/model"
	"go-finance/internal/service"
	"go-finance/internal/store"
	"net/http"
	"time"
)

type FinanceHandler struct {
	Store *store.PostgresStore
}

func NewFinanceHandler(s *store.PostgresStore) *FinanceHandler {
	return &FinanceHandler{Store: s}
}

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// CreateTransaction nhận JSON từ Bot
func (h *FinanceHandler) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	var req model.TransactionCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	// Chuyển đổi tỷ giá nếu không phải VND (Logic đơn giản hóa, thực tế cần gọi Service lấy giá live)
	// Ở đây giả định Bot gửi amount là số lượng gốc (ví dụ 0.1 BTC)
	convertedAmount := req.Amount
	originalAmount := req.Amount

	// Lấy tỷ giá hiện tại để quy đổi ra VND lưu vào DB cho thống kê Cashflow
	rates, _ := service.GetMetalPrices()
	switch req.Currency {
	case "USD":
		convertedAmount = req.Amount * rates.UsdVND
	case "GOLD":
		convertedAmount = req.Amount * rates.VnSJC
	case "BTC":
		convertedAmount = req.Amount * rates.BtcVND
	default:
		// VND
		originalAmount = req.Amount
	}

	t := model.Transaction{
		UserID:         req.UserID,
		Type:           req.Type,
		Amount:         convertedAmount,
		OriginalAmount: originalAmount,
		Note:           req.Note,
		Currency:       req.Currency,
		Category:       req.Category,
	}

	if err := h.Store.Create(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GenerateReport tạo báo cáo
func (h *FinanceHandler) GenerateReport(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	period := r.URL.Query().Get("period")

	now := time.Now()
	var startDate time.Time

	if period == "week" {
		// Lấy đầu tuần (T2)
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startDate = now.AddDate(0, 0, -weekday+1)
	} else {
		// Đầu tháng
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}

	txs, err := h.Store.GetByPeriod(userID, startDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Logic tổng hợp báo cáo
	report := model.ReportOutput{
		Period:            period,
		StartDate:         startDate.Format("2006-01-02"),
		ExpenseByCategory: make(map[string]float64),
		Assets:            make(map[string]model.AssetDetail),
	}

	currentRates, _ := service.GetMetalPrices()

	for _, t := range txs {
		switch t.Type {
		case "thu":
			report.TotalIncome += t.Amount
		case "chi":
			report.TotalExpense += t.Amount
			report.ExpenseByCategory[t.Category] += t.Amount
		case "tiet_kiem":
			report.TotalSavingsVND += t.Amount

			// Cộng dồn tài sản
			if t.Currency != "VND" {
				asset := report.Assets[t.Currency]
				asset.Quantity += t.OriginalAmount

				// Cập nhật giá trị hiện tại theo tỷ giá mới nhất
				rate := 1.0
				switch t.Currency {
				case "USD":
					rate = currentRates.UsdVND
				case "GOLD":
					rate = currentRates.VnSJC
				case "BTC":
					rate = currentRates.BtcVND
				}

				asset.Rate = rate
				asset.CurrentVND = asset.Quantity * rate
				report.Assets[t.Currency] = asset
			}
		}
	}

	// Tính tổng giá trị tài sản
	for _, a := range report.Assets {
		report.TotalAssetsVND += a.CurrentVND
	}

	report.Balance = report.TotalIncome - report.TotalExpense - report.TotalSavingsVND
	jsonResponse(w, http.StatusOK, report)
}

// GetPrices trả về giá vàng/bạc
func (h *FinanceHandler) GetPrices(w http.ResponseWriter, r *http.Request) {
	rates, err := service.GetMetalPrices()
	if err != nil {
		http.Error(w, "Error fetching prices", http.StatusInternalServerError)
		return
	}

	// Constants
	const OunceToTael = 1.20565 // 1 Ounce quốc tế = 1.20565 Lượng

	// 1. Tính toán Vàng (GOLD)
	// Giá thế giới quy đổi (VNĐ/Lượng) = Giá $ * Tỷ giá USD * Hệ số chuyển đổi
	worldGoldVND := rates.GoldUSD * rates.UsdVND * OunceToTael
	rates.GoldDiff = rates.VnSJC - worldGoldVND

	// 2. Tính toán Bạc (SILVER)
	worldSilverVND := rates.SilverUSD * rates.UsdVND * OunceToTael
	rates.SilverDiff = rates.VnSilver - worldSilverVND

	jsonResponse(w, http.StatusOK, rates)
}
