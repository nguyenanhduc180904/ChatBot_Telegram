package model

import "time"

// Transaction tương ứng với bảng trong DB
type Transaction struct {
	ID             int       `json:"id"`
	UserID         string    `json:"user_id"`
	Type           string    `json:"type"`   // thu, chi, tiet_kiem
	Amount         float64   `json:"amount"` // Giá trị quy đổi VND
	Note           string    `json:"note"`
	Category       string    `json:"category"` // Có thể rỗng
	CreatedAt      time.Time `json:"created_at"`
	Currency       string    `json:"currency"`        // VND, USD, BTC, GOLD
	OriginalAmount float64   `json:"original_amount"` // Số lượng gốc
}

// TransactionCreate DTO cho input
type TransactionCreate struct {
	UserID   string  `json:"user_id"`
	Type     string  `json:"type"`
	Amount   float64 `json:"amount"`
	Note     string  `json:"note"`
	Currency string  `json:"currency"` // Mặc định VND
	Category string  `json:"category"`
}

// ReportOutput DTO cho báo cáo
type ReportOutput struct {
	Period            string                 `json:"period"`
	StartDate         string                 `json:"start_date"`
	TotalIncome       float64                `json:"total_income"`
	TotalExpense      float64                `json:"total_expense"`
	TotalSavingsVND   float64                `json:"total_savings_vnd"`
	Balance           float64                `json:"balance"`
	ExpenseByCategory map[string]float64     `json:"expense_by_category"`
	Assets            map[string]AssetDetail `json:"assets"`
	TotalAssetsVND    float64                `json:"total_assets_vnd"`
}

type AssetDetail struct {
	Quantity   float64 `json:"quantity"`
	CurrentVND float64 `json:"current_vnd"`
	Rate       float64 `json:"rate"`
}

// ExchangeRates DTO cho giá cả
type ExchangeRates struct {
	GoldUSD    float64 `json:"gold_usd"`
	SilverUSD  float64 `json:"silver_usd"`
	VnSJC      float64 `json:"vn_sjc"`
	UsdVND     float64 `json:"usd_vnd"`
	VnSilver   float64 `json:"vn_silver_est"`
	GoldDiff   float64 `json:"gold_diff"`   // Chênh lệch
	SilverDiff float64 `json:"silver_diff"` // Chênh lệch
	BtcVND     float64 `json:"btc_vnd"`
}
