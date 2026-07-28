package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"cryptorum/internal/auth"
)

const maxPasswordBytes = 4096

func main() {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, maxPasswordBytes+1))
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to read password:", err)
		os.Exit(1)
	}
	if len(data) > maxPasswordBytes {
		fmt.Fprintln(os.Stderr, "password is too long")
		os.Exit(1)
	}

	password := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if password == "" {
		fmt.Fprintln(os.Stderr, "provide a non-empty password on standard input")
		os.Exit(1)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to hash password:", err)
		os.Exit(1)
	}
	fmt.Println(hash)
}
