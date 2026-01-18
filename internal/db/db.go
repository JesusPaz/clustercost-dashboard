package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite" // Register SQLite driver
)

type Store struct {
	db *sql.DB
}

func New(storagePath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(storagePath), 0750); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", storagePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate db: %w", err)
	}

	// Seed default admin if user table is empty
	if err := s.SeedAdmin(); err != nil {
		log.Printf("[DB] Warning: failed to seed admin: %v", err)
	}

	return s, nil
}

func (s *Store) migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *Store) SeedAdmin() error {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	username := "admin"
	// Default password: "password"
	// In production, force user to change this or pass via ENV
	password := "password"
	if envPass := os.Getenv("ADMIN_PASSWORD"); envPass != "" {
		password = envPass
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = s.db.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", username, string(hash))
	if err == nil {
		log.Printf("[DB] Created initial admin user: '%s' with password from env (or default 'password')", username)
	}
	return err
}

func (s *Store) GetUserPasswordHash(username string) (string, error) {
	var hash string
	err := s.db.QueryRow("SELECT password_hash FROM users WHERE username = ?", username).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil // User not found
	}
	return hash, err
}

func (s *Store) Close() error {
	return s.db.Close()
}
