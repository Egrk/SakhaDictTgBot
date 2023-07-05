package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	dictURL        = "https://igi.ysn.ru/btsja/index.php?data1=&talww=1"
	helpMessage    = "Всё просто: отправляешь мне слово на якутском языке, я выдаю его значение. Знаком '+' в начале можно получить html-файл с полным пояснение слова с примерами. Пример запроса: '+Саха'"
	startMessage   = "Вас приветствует бот Большого толкового словаря якутского языка. Данные берутся из его электронной версии по ссылке: https://igi.ysn.ru/btsja/index.php. Бот некоммерческий и создан только для удобного взаимодействия со словарем. Небольшая инструкция: /help"
	defaultMessage = "Такой команды не знаю :("
	serverErrorMessage = "Ошибка сервера, попробуйте попытку позже"
	htmlStartElement = "<div class = 'text'>"
	htmlEndElement = "</div>"
)

func main() {
	localMode := flag.Bool("dev", false, "run in local machine")
	flag.Parse()
	var updates tgbotapi.UpdatesChannel
	var bot *tgbotapi.BotAPI
	if *localMode {
		config, _ := loadConfig(".")
		var err error
		bot, err = tgbotapi.NewBotAPI(config.ApiKey)
		if err != nil {
			log.Panic(err)
		}
		log.Printf("Authorized on account %s", bot.Self.UserName)
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		updates = bot.GetUpdatesChan(u)
	} else {
		apiKey := os.Getenv("API_KEY")
		port := os.Getenv("PORT")
		host := os.Getenv("HOST")
		if apiKey == "" || port == "" || host == "" {
			log.Fatal("API_KEY, PORT and HOST must be set")
			return
		}

		var err error
		bot, err = tgbotapi.NewBotAPI(apiKey)
		if err != nil {
			log.Panic(err)
		}
		log.Printf("Authorized on account %s", bot.Self.UserName)

		addr := "0.0.0.0:" + port
		go http.ListenAndServe(addr, nil)
		log.Println("Listenning on port: " + port)
		
		webhook, err := tgbotapi.NewWebhook(host + bot.Token)
		if err != nil {
			log.Fatal(err)
		}

		_, err = bot.Request(webhook)
		if err != nil {
			log.Fatal(err)
		}
		updates = bot.ListenForWebhook("/" + bot.Token)
	}

	urlAddress, _ := url.Parse(dictURL)
	log.Println("Configs setted, starting listening")
	for update := range updates {
		if update.Message == nil {
			continue
		}
		fullTextMdFile := false
		log.Println("User: ", update.Message.Chat.FirstName, update.Message.Chat.LastName, "UserName: ", update.Message.Chat.UserName)
		if update.Message.IsCommand() {
			log.Println("Send command: ", update.Message.Command())
			switch update.Message.Command() {
			case "help":
				sendMessage(helpMessage, update.Message.Chat.ID, bot)
				continue
			case "start":
				sendMessage(startMessage, update.Message.Chat.ID, bot)
				continue
			default:
				sendMessage(defaultMessage, update.Message.Chat.ID, bot)
				continue
			}
		}
		searchWord := update.Message.Text
		log.Println("Searching for word: ", searchWord)
		if searchWord[0] == '+' {
			fullTextMdFile = true
			searchWord = strings.TrimSpace(searchWord[1:])
		}
		query := urlAddress.Query()
		query.Set("data1", searchWord)
		urlAddress.RawQuery = query.Encode()
		resp, err := http.Get(fmt.Sprint(urlAddress, searchWord))
		if err != nil {
			log.Fatal("error happened ", err)
			continue
		}
		if resp.StatusCode != 200 {
			log.Println("Server error, status code: ", resp.StatusCode)
			sendMessage(serverErrorMessage, update.Message.Chat.ID, bot)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal("error happened ", err)
		}
		resp.Body.Close()
		pack := pack{
			messageId: update.Message.Chat.ID,
			bot: bot,
		}
		if len(body) < 14000 {
			sendMessage("Слово не найдено", update.Message.Chat.ID, bot)
			continue
		}
		if fullTextMdFile {
			sendHtmlChunkWithText(body[11477:], searchWord, update.Message.Chat.ID, bot)
		} else {
			go parseHtmlBody(body[11477:], pack)
		}
	}
}
