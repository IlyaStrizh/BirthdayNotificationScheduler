package main

import (
	h "birthdayGreetings/internal/handle"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	tb "gopkg.in/telebot.v3"
)

func init() {
	err := godotenv.Load("../configs/.env")
	if err != nil {
		log.Fatal("Ошибка загрузки файла .env")
	}
}

func main() {
	h := h.NewHandle()
	defer h.CloseDB()

	b := RunTelegramBot()

	b.Handle("/start", h.BotStart)
	b.Handle("/help", h.BotHelp)
	b.Handle("/login", h.Login)
	b.Handle("/subscribe", h.SubscribeToNotifications)
	b.Handle("/unsubscribe", h.UnsubscribeFromNotifications)
	b.Handle("/list", h.List)
	b.Handle("/subscribed", h.Subscribed)

	// Обработка ответов
	b.Handle(tb.OnText, h.WaitUserResponse)

	go h.Scheduler(b)
	log.Println("Бот запущен...")
	b.Start()
}

// RunTelegramBot функция для запуска Telegram Bot
func RunTelegramBot() *tb.Bot {
	bot, err := tb.NewBot(tb.Settings{
		Token:  os.Getenv("BOT_TOKEN"),
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		log.Fatalf("Бот создать не удалось: %s", err)
	}

	return bot
}
