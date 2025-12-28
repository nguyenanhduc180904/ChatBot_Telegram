package tests

import (
	"go-finance/internal/model"
	"go-finance/internal/service"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTransactionText(t *testing.T) {
	// Định nghĩa các test case
	tests := []struct {
		name     string
		input    string
		expected []model.TransactionCreate
	}{
		// =================================================================
		// NHÓM 1: CÁC TRƯỜNG HỢP CÚ PHÁP CƠ BẢN (BASIC SYNTAX)
		// =================================================================
		{
			name:  "Chi tiêu thông thường (k)",
			input: "chi 50k ăn sáng",
			expected: []model.TransactionCreate{
				{Type: "chi", Amount: 50000, Note: "ăn sáng", Currency: "VND", Category: "ăn uống"},
			},
		},
		{
			name:  "Thu nhập thông thường (m)",
			input: "thu 10m lương",
			expected: []model.TransactionCreate{
				{Type: "thu", Amount: 10000000, Note: "lương", Currency: "VND", Category: ""},
			},
		},
		{
			name:  "Tiết kiệm (viết tắt tk, không note)",
			input: "tk 2m",
			expected: []model.TransactionCreate{
				{Type: "tiet_kiem", Amount: 2000000, Note: "", Currency: "VND", Category: ""},
			},
		},
		{
			name:  "Tiết kiệm (viết đầy đủ)",
			input: "tiết kiệm 500k",
			expected: []model.TransactionCreate{
				{Type: "tiet_kiem", Amount: 500000, Note: "", Currency: "VND", Category: ""},
			},
		},

		// =================================================================
		// NHÓM 2: CÚ PHÁP DẤU CỘNG/TRỪ (SIGN SYNTAX)
		// =================================================================
		{
			name:  "Dùng dấu trừ (-) đại diện cho Chi",
			input: "- 20k tiền nước",
			expected: []model.TransactionCreate{
				{Type: "chi", Amount: 20000, Note: "tiền nước", Currency: "VND", Category: "sinh hoạt"},
			},
		},
		{
			name:  "Dùng dấu cộng (+) đại diện cho Thu",
			input: "+ 500k thưởng nóng",
			expected: []model.TransactionCreate{
				{Type: "thu", Amount: 500000, Note: "thưởng nóng", Currency: "VND", Category: ""},
			},
		},

		// =================================================================
		// NHÓM 3: ĐỊNH DẠNG SỐ VÀ ĐƠN VỊ (FORMATTING)
		// =================================================================
		{
			name:  "Số thập phân dùng dấu chấm (1.5m)",
			input: "chi 1.5m tiền trọ",
			expected: []model.TransactionCreate{
				{Type: "chi", Amount: 1500000, Note: "tiền trọ", Currency: "VND", Category: "khác"},
			},
		},
		{
			name:  "Số thập phân dùng dấu phẩy (1,5m)",
			input: "chi 1,5m tiền trọ",
			expected: []model.TransactionCreate{
				{Type: "chi", Amount: 1500000, Note: "tiền trọ", Currency: "VND", Category: "khác"},
			},
		},
		{
			name:  "Số thường không đơn vị (mặc định là số trần)",
			input: "chi 50000 trà sữa",
			expected: []model.TransactionCreate{
				{Type: "chi", Amount: 50000, Note: "trà sữa", Currency: "VND", Category: "ăn uống"},
			},
		},

		// =================================================================
		// NHÓM 4: TIỀN TỆ KHÁC (CURRENCIES - CHỈ ÁP DỤNG CHO TIẾT KIỆM)
		// =================================================================
		// Theo logic code: Thu/Chi chỉ chấp nhận VND. Tiết kiệm chấp nhận ngoại tệ.
		{
			name:  "Tiết kiệm USD",
			input: "tk 100 usd",
			expected: []model.TransactionCreate{
				{Type: "tiet_kiem", Amount: 100, Note: "", Currency: "USD", Category: ""},
			},
		},
		{
			name:  "Tiết kiệm Bitcoin (BTC)",
			input: "tk 0.5 btc",
			expected: []model.TransactionCreate{
				{Type: "tiet_kiem", Amount: 0.5, Note: "", Currency: "BTC", Category: ""},
			},
		},
		{
			name:  "Tiết kiệm Vàng (chỉ vàng)",
			input: "tk 5 chỉ vàng",
			expected: []model.TransactionCreate{
				{Type: "tiet_kiem", Amount: 5, Note: "", Currency: "GOLD", Category: ""},
			},
		},

		// =================================================================
		// NHÓM 5: NHIỀU GIAO DỊCH (MULTIPLE TRANSACTIONS)
		// =================================================================
		{
			name:  "Hai lệnh trên một dòng ngăn cách bởi dấu phẩy",
			input: "chi 50k ăn trưa, + 200k bán đồ cũ",
			expected: []model.TransactionCreate{
				{Type: "chi", Amount: 50000, Note: "ăn trưa", Currency: "VND", Category: "ăn uống"},
				{Type: "thu", Amount: 200000, Note: "bán đồ cũ", Currency: "VND", Category: ""},
			},
		},
		{
			name:  "Hai lệnh ngăn cách bởi xuống dòng",
			input: "chi 30k cafe\ntk 100 usd",
			expected: []model.TransactionCreate{
				{Type: "chi", Amount: 30000, Note: "cafe", Currency: "VND", Category: "ăn uống"},
				{Type: "tiet_kiem", Amount: 100, Note: "", Currency: "USD", Category: ""},
			},
		},

		// =================================================================
		// NHÓM 6: LOGIC VALIDATION CỦA CODE (BUSINESS RULES)
		// =================================================================
		// Code có các rule check và bỏ qua (continue) nếu không thỏa mãn.
		// Test này đảm bảo logic bỏ qua hoạt động đúng (trả về danh sách rỗng).

		{
			name:     "Bỏ qua: Tiết kiệm nhưng lại có Note",
			input:    "tk 100k tiền để dành", // Rule: Tiết kiệm KHÔNG được có note
			expected: nil,
		},
		{
			name:     "Bỏ qua: Thu/Chi nhưng thiếu Note",
			input:    "chi 50k", // Rule: Thu/Chi BẮT BUỘC có note
			expected: nil,
		},
		{
			name:     "Bỏ qua: Thu/Chi bằng ngoại tệ",
			input:    "chi 10 usd mua game", // Rule: Thu/Chi chỉ dùng VND
			expected: nil,
		},
		{
			name:     "Bỏ qua: Số tiền bằng 0",
			input:    "chi 0k test", // Code check: if val <= 0 { continue }
			expected: nil,
		},
		{
			name:     "Bỏ qua: Cú pháp không hiểu",
			input:    "hello world",
			expected: nil,
		},

		// =================================================================
		// NHÓM 7: KIỂM TRA PHÂN LOẠI TỰ ĐỘNG (AUTO CATEGORIZATION)
		// =================================================================
		{
			name:  "Phân loại: Ăn uống (từ khóa 'phở')",
			input: "chi 40k phở bò",
			expected: []model.TransactionCreate{
				{Type: "chi", Amount: 40000, Note: "phở bò", Currency: "VND", Category: "ăn uống"},
			},
		},
		{
			name:  "Phân loại: Sinh hoạt (từ khóa 'xăng')",
			input: "chi 100k đổ xăng",
			expected: []model.TransactionCreate{
				{Type: "chi", Amount: 100000, Note: "đổ xăng", Currency: "VND", Category: "sinh hoạt"},
			},
		},
		{
			name:  "Phân loại: Hưởng thụ (từ khóa 'massage')",
			input: "chi 300k đi massage",
			expected: []model.TransactionCreate{
				{Type: "chi", Amount: 300000, Note: "đi massage", Currency: "VND", Category: "hưởng thụ"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.ParseTransactionText(tt.input)

			assert.NoError(t, err)

			if tt.expected == nil {
				assert.Empty(t, got, "Mong đợi danh sách rỗng cho input không hợp lệ")
			} else {
				assert.Equal(t, len(tt.expected), len(got), "Số lượng giao dịch không khớp")
				for i, want := range tt.expected {
					assert.Equal(t, want.Type, got[i].Type)
					assert.Equal(t, want.Amount, got[i].Amount)
					assert.Equal(t, want.Note, got[i].Note)
					assert.Equal(t, want.Currency, got[i].Currency)
					assert.Equal(t, want.Category, got[i].Category)
				}
			}
		})
	}
}
