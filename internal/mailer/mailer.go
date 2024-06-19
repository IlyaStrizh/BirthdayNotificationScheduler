package mailer

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strconv"

	"gopkg.in/gomail.v2"
)

// SendPasswordToEmail функция для отправки пароля на email
func SendPasswordToEmail(email string) (string, error) {
	// Генерируем случайный пароль
	password := generateRandomPassword(5)

	mailer, err := makeMailer()
	if err != nil {
		return "", fmt.Errorf("не удалось создать mailer. %v", err)
	}

	recipient := "MAILER_RECIPIENT"
	if recipient == "" {
		return "", fmt.Errorf("пустой получатель")
	}

	m := gomail.NewMessage()
	m.SetHeader("From", "your-email@example.com")
	m.SetHeader("To", email)
	m.SetHeader("Subject", "Временный пароль")
	m.SetBody("text/plain", fmt.Sprintf("Ваш временный пароль: %s", password))

	err = mailer.DialAndSend(m)
	if err != nil {
		return "", fmt.Errorf("не удалось отправить почту: %v", err)
	}

	return password, nil
}

// generateRandomPassword функция для генерации случайного пароля
func generateRandomPassword(length int) string {
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	password := make([]byte, length)
	for i := range password {
		password[i] = charset[rand.Intn(len(charset))]
	}
	return string(password)
}

// makeMailer функция создания mailer-а
func makeMailer() (*gomail.Dialer, error) {
	smtp := os.Getenv("SMTP")
	port, _ := strconv.Atoi(os.Getenv("SMTP_PORT"))

	name := os.Getenv("SMTP_NAME")
	pwd := os.Getenv("SMTP_PASSWORD")
	if port == 0 || smtp == "" && name == "" {
		return nil, errors.New("недопустимые параметры  mailer")
	}

	return gomail.NewDialer(smtp, port, name, pwd), nil
}
