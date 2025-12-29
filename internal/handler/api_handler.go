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

// CreateTransaction godoc
// @Summary      Tạo giao dịch mới
// @Description  API nhận dữ liệu giao dịch. Hỗ trợ tự động quy đổi tỷ giá nếu dùng ngoại tệ.
// @Description
// @Description  ### 💡 HƯỚNG DẪN TEST NHANH (Copy JSON bên dưới dán vào ô Request):
// @Description
// @Description  **1️⃣ Trường hợp: CHI TIÊU (VND)**
// @Description  ```json
// @Description  {
// @Description      "user_id": "123456789",
// @Description      "type": "chi",
// @Description      "amount": 55000,
// @Description      "note": "Ăn trưa cơm tấm",
// @Description      "category": "ăn uống",
// @Description      "currency": "VND"
// @Description  }
// @Description  ```
// @Description
// @Description  **2️⃣ Trường hợp: THU NHẬP (VND)**
// @Description  ```json
// @Description  {
// @Description      "user_id": "123456789",
// @Description      "type": "thu",
// @Description      "amount": 15000000,
// @Description      "note": "Lương tháng 12",
// @Description      "currency": "VND"
// @Description  }
// @Description  ```
// @Description
// @Description  **3️⃣ Trường hợp: TIẾT KIỆM (Vàng/Ngoại tệ)**
// @Description  _(Hệ thống sẽ tự quy đổi ra VND theo tỷ giá hiện tại)_
// @Description  ```json
// @Description  {
// @Description      "user_id": "123456789",
// @Description      "type": "tiet_kiem",
// @Description      "amount": 2,
// @Description      "note": "Mua 2 chỉ vàng tích trữ",
// @Description      "currency": "GOLD"
// @Description  }
// @Description  ```
// @Tags         Transactions
// @Accept       json
// @Produce      json
// @Param        payload  body      model.TransactionCreate  true  "Dữ liệu giao dịch"
// @Success      200      {object}  map[string]string        "Thành công"
// @Failure      400      {string}  string                   "Lỗi dữ liệu đầu vào"
// @Failure      500      {string}  string                   "Lỗi Server"
// @Router       /transactions [post]
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

	rates := service.GetCurrentRates()

	switch req.Currency {
	case "USD":
		convertedAmount = req.Amount * rates.UsdVND
	case "GOLD":
		convertedAmount = req.Amount * rates.VnSJC
	case "BTC":
		convertedAmount = req.Amount * rates.BtcVND
	default: // VND hoặc loại khác
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

// GenerateReport godoc
// @Summary      Xuất báo cáo tài chính
// @Description  Tổng hợp thu/chi, tính toán số dư và định giá tài sản tích lũy theo thời gian thực.
// @Tags         Reports
// @Accept       json
// @Produce      json
// @Param        user_id  query     string  true  "ID người dùng Telegram (VD: 123456789)"
// @Param        period   query     string  true  "Kỳ báo cáo: 'week' (tuần này) hoặc 'month' (tháng này)"
// @Success      200      {object}  model.ReportOutput
// @Failure      500      {string}  string  "Lỗi Server"
// @Router       /report [get]
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
		monday := now.AddDate(0, 0, -weekday+1)
		startDate = time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, monday.Location())
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

	currentRates := service.GetCurrentRates()

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

// GetPrices godoc
// @Summary      Lấy tỷ giá thị trường
// @Description  Lấy giá Vàng, Bạc, Bitcoin, USD từ các nguồn bên ngoài (CoinGecko, GoldAPI...).
// @Tags         Market Data
// @Accept       json
// @Produce      json
// @Success      200  {object}  model.ExchangeRates
// @Failure      500  {string}  string  "Lỗi không lấy được dữ liệu"
// @Router       /market-rates [get]
func (h *FinanceHandler) GetPrices(w http.ResponseWriter, r *http.Request) {
	// [TỐI ƯU] Thay vì gọi service.GetMetalPrices() (tốn 3-5s), ta gọi service.GetCurrentRates() để lấy dữ liệu đã cache
	rates := service.GetCurrentRates()

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
