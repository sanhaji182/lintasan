package main

import (
	"crypto/rand"
	"crypto/sha512"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

const (
	passwordIterations = 200_000
	saltBytes          = 32
)

func hashWithSaltIterations(password string, salt []byte, iterations int) []byte {
	h := sha512.New()
	h.Write(salt)
	h.Write([]byte(password))
	hash := h.Sum(nil)
	for i := 1; i < iterations; i++ {
		h.Reset()
		h.Write(hash)
		h.Write(salt)
		h.Write([]byte(password))
		hash = h.Sum(nil)
	}
	return hash
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := hashWithSaltIterations(password, salt, passwordIterations)
	return fmt.Sprintf("$sha512$%d$%s$%s",
		passwordIterations,
		hex.EncodeToString(salt),
		hex.EncodeToString(hash),
	), nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run . <new-password>")
		os.Exit(1)
	}
	newPass := os.Args[1]

	hash, err := hashPassword(newPass)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash error: %v\n", err)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite3", "/home/ubuntu/lintasan-go/data/lintasan.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "db open: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	result, err := db.Exec("UPDATE users SET password_hash = ?, must_change_password = 0 WHERE username = 'admin'", hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "update error: %v\n", err)
		os.Exit(1)
	}
	rows, _ := result.RowsAffected()
	fmt.Printf("OK, rows: %d\n", rows)
}
