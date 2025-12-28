package store

import (
	"database/sql"
	"go-finance/internal/model"
	"time"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) InitSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS transactions (
		id SERIAL PRIMARY KEY,
		user_id VARCHAR(50) NOT NULL,
		type VARCHAR(20) NOT NULL,
		amount FLOAT NOT NULL,
		note TEXT,
		category VARCHAR(50),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		currency VARCHAR(10) DEFAULT 'VND',
		original_amount FLOAT DEFAULT 0.0
	);`
	_, err := s.db.Exec(query)
	return err
}

func (s *PostgresStore) Create(t model.Transaction) error {
	query := `
		INSERT INTO transactions (user_id, type, amount, note, category, currency, original_amount, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	// Tự động phân loại đơn giản nếu chưa có category
	category := t.Category
	if category == "" && t.Type == "chi" {
		category = "khác" // Logic đơn giản hóa
	}

	_, err := s.db.Exec(query, t.UserID, t.Type, t.Amount, t.Note, category, t.Currency, t.OriginalAmount, time.Now())
	return err
}

func (s *PostgresStore) GetByPeriod(userID string, startDate time.Time) ([]model.Transaction, error) {
	query := `
		SELECT id, user_id, type, amount, note, category, created_at, currency, original_amount
		FROM transactions
		WHERE user_id = $1 AND created_at >= $2
	`
	rows, err := s.db.Query(query, userID, startDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []model.Transaction
	for rows.Next() {
		var t model.Transaction
		var note, cat, curr sql.NullString // Handle nulls safely

		if err := rows.Scan(&t.ID, &t.UserID, &t.Type, &t.Amount, &note, &cat, &t.CreatedAt, &curr, &t.OriginalAmount); err != nil {
			return nil, err
		}
		t.Note = note.String
		t.Category = cat.String
		t.Currency = curr.String
		if t.Currency == "" {
			t.Currency = "VND"
		}
		txs = append(txs, t)
	}
	return txs, nil
}

// GetAllUserIDs lấy danh sách tất cả user_id duy nhất
func (s *PostgresStore) GetAllUserIDs() ([]string, error) {
	query := `SELECT DISTINCT user_id FROM transactions`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			continue
		}
		userIDs = append(userIDs, uid)
	}
	return userIDs, nil
}
