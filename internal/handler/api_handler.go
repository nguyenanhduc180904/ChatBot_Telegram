package handler

import (
	"encoding/json"
	"go-finance/internal/model"
	"go-finance/internal/service"
	"go-finance/internal/store"
	"log"
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
		log.Printf("[API ERROR] Decode JSON failed: %v", err) // [Update] Log lỗi input
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	log.Printf("[API INFO] Received transaction request: %+v", req) // [Update] Log request nhận được

	convertedAmount := req.Amount
	originalAmount := req.Amount

	rates, err := service.GetMetalPrices()
	if err != nil {
		log.Printf("[API WARN] Could not fetch metal prices: %v", err) // [Update] Log cảnh báo
	}

	switch req.Currency {
	case "USD":
		convertedAmount = req.Amount * rates.UsdVND
	case "GOLD":
		convertedAmount = req.Amount * rates.VnSJC
	case "BTC":
		convertedAmount = req.Amount * rates.BtcVND
	default:
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
		log.Printf("[API ERROR] DB Create failed: %v", err) // [Update] Log lỗi DB
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GenerateReport tạo báo cáo
func (h *FinanceHandler) GenerateReport(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	period := r.URL.Query().Get("period")

	log.Printf("[API INFO] GenerateReport for User: %s, Period: %s", userID, period) // [Update]

	now := time.Now()
	var startDate time.Time

	if period == "week" {
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startDate = now.AddDate(0, 0, -weekday+1)
	} else {
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}

	txs, err := h.Store.GetByPeriod(userID, startDate)
	if err != nil {
		log.Printf("[API ERROR] DB GetByPeriod failed: %v", err) // [Update]
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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
			if t.Currency != "VND" {
				asset := report.Assets[t.Currency]
				asset.Quantity += t.OriginalAmount
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
		log.Printf("[API ERROR] GetPrices failed: %v", err) // [Update]
		http.Error(w, "Error fetching prices", http.StatusInternalServerError)
		return
	}

	const OunceToTael = 1.20565
	worldGoldVND := (rates.GoldUSD * rates.UsdVND * OunceToTael)
	rates.GoldDiff = rates.VnSJC*10 - worldGoldVND

	worldSilverVND := rates.SilverUSD * rates.UsdVND * OunceToTael
	rates.SilverDiff = rates.VnSilver - worldSilverVND

	jsonResponse(w, http.StatusOK, rates)
}

// GetUsers trả về danh sách user_id
func (h *FinanceHandler) GetUsers(w http.ResponseWriter, r *http.Request) {
	userIDs, err := h.Store.GetAllUserIDs()
	if err != nil {
		log.Printf("[API ERROR] GetUsers failed: %v", err)
		http.Error(w, "Error fetching users", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, http.StatusOK, userIDs)
}
