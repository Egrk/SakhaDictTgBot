package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/patrickmn/go-cache"
)

const (
	dictURL            = "https://igi.ysn.ru/btsja/index.php?data1=&talww=1"
	helpMessage        = "Всё просто: отправляешь мне слово на якутском языке, я выдаю его значение. Знаки '>' и '<' листают страницы значения слова, знаки '=>' и '<=' переходят по значениям слова. Знаком '+' в начале можно получить html-файл с полным пояснение слова с примерами. Пример запроса: '+Саха'"
	startMessage       = "Вас приветствует бот Большого толкового словаря якутского языка. Данные берутся из его электронной версии по ссылке: https://igi.ysn.ru/btsja/index.php. Бот некоммерческий и создан только для удобного взаимодействия со словарем. Небольшая инструкция: /help"
	defaultMessage     = "Такой команды не знаю :("
	serverErrorMessage = "Ошибка сервера, попробуйте попытку позже"
	htmlStartElement   = "<div class = 'text'>"
	htmlEndElement     = "</div>"
)

var bot *tgbotapi.BotAPI
var memoryCache *memCache

func newCache() *memCache {
	cache := cache.New(30*time.Minute, 15*time.Minute)
	return &memCache{
		cachedWords: cache,
	}
}

type cachedWord struct {
	textList       []string
	chaptersNumber []int
}

type memCache struct {
	cachedWords *cache.Cache
}

func (c *memCache) read(id string) (item cachedWord, ok bool) {
	word, ok := c.cachedWords.Get(id)
	if ok {
		log.Println("From cache")
		return word.(cachedWord), true
	}
	return cachedWord{}, false
}

func (c *memCache) update(id string, word cachedWord) {
	c.cachedWords.Set(id, word, cache.DefaultExpiration)
}

func getKeyboard(leftPageData, rightPageData, prevChapterData, nextChapterData string) (tgbotapi.InlineKeyboardMarkup, bool) {
	var pageLine []tgbotapi.InlineKeyboardButton
	var chapterLine []tgbotapi.InlineKeyboardButton
	if leftPageData != "" {
		pageLine = append(pageLine, tgbotapi.NewInlineKeyboardButtonData("<", leftPageData))
	}
	if rightPageData != "" {
		pageLine = append(pageLine, tgbotapi.NewInlineKeyboardButtonData(">", rightPageData))
	}
	if prevChapterData != "" {
		chapterLine = append(chapterLine, tgbotapi.NewInlineKeyboardButtonData("<=", prevChapterData))
	}
	if nextChapterData != "" {
		chapterLine = append(chapterLine, tgbotapi.NewInlineKeyboardButtonData("=>", nextChapterData))
	}

	var keyboardRows [][]tgbotapi.InlineKeyboardButton
	if len(pageLine) != 0 {
		keyboardRows = append(keyboardRows, pageLine)
	}
	if len(chapterLine) != 0 {
		keyboardRows = append(keyboardRows, chapterLine)
	}

	keyboardMarkup := tgbotapi.NewInlineKeyboardMarkup(
		keyboardRows...,
	)
	if len(keyboardMarkup.InlineKeyboard) == 0 {
		return keyboardMarkup, false
	}

	return keyboardMarkup, true
}

func main() {
	localMode := flag.Bool("dev", false, "run in local machine")
	flag.Parse()
	var updates tgbotapi.UpdatesChannel
	if *localMode {
		var err error
		config, err := loadConfig(".")
		if err != nil {
			log.Fatalf("Error on config load: %s", err)
		}
		bot, err = tgbotapi.NewBotAPI(config.ApiKey)
		if err != nil {
			log.Fatalf("Error on telegram bot setting: %s", err)
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
		}

		var err error
		bot, err = tgbotapi.NewBotAPI(apiKey)
		if err != nil {
			log.Fatalf("Error on telegram bot setting: %s", err)
		}
		log.Printf("Authorized on account %s", bot.Self.UserName)

		addr := "0.0.0.0:" + port
		go http.ListenAndServe(addr, nil)
		log.Println("Listenning on port: " + port)

		webhook, err := tgbotapi.NewWebhook(host + bot.Token)
		if err != nil {
			log.Fatalf("Error on web hook setting: %s", err)
		}

		_, err = bot.Request(webhook)
		if err != nil {
			log.Fatalf("Telegram bot respond error: %s", err)
		}
		updates = bot.ListenForWebhook("/" + bot.Token)
	}
	memoryCache = newCache()
	urlAddress, _ := url.Parse(dictURL)
	log.Println("Configs setted, start listening")
	var downstream = make(chan pack, 20)
	go balancer(downstream)
	for update := range updates {
		if update.Message != nil {
			fullTextMdFile := false
			log.Println("User: ", update.Message.Chat.FirstName, update.Message.Chat.LastName, "UserName: ", update.Message.Chat.UserName)
			if update.Message.IsCommand() {
				log.Println("Send command: ", update.Message.Command())
				switch update.Message.Command() {
				case "help":
					sendText(helpMessage, update.Message.Chat.ID)
					continue
				case "start":
					sendText(startMessage, update.Message.Chat.ID)
					continue
				default:
					sendText(defaultMessage, update.Message.Chat.ID)
					continue
				}
			}
			searchWord := update.Message.Text
			log.Println("Searching for word: ", searchWord)
			if searchWord[0] == '+' {
				fullTextMdFile = true
				searchWord = strings.TrimSpace(searchWord[1:])
			}
			if !fullTextMdFile {
				cachedData, ok := memoryCache.read(searchWord)
				if ok {
					log.Println("Found in cache")
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, cachedData.textList[0])
					keyboard, ok := getKeyboard(getWordKeyboardData(searchWord, &cachedData, 0))
					if ok {
						msg.ReplyMarkup = keyboard
					}
					if _, err := bot.Send(msg); err != nil {
						log.Fatal("error happened ", err)
					}
					continue
				}
			}
			query := urlAddress.Query()
			query.Set("data1", searchWord)
			urlAddress.RawQuery = query.Encode()
			resp, err := http.Get(urlAddress.String())
			if err != nil {
				log.Fatal("error happened ", err)
				continue
			}
			if resp.StatusCode != 200 {
				log.Println("Server error, status code: ", resp.StatusCode)
				sendText(serverErrorMessage, update.Message.Chat.ID)
				continue
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Fatal("error happened ", err)
			}
			resp.Body.Close()
			if len(body) < 14000 {
				sendText("Слово не найдено", update.Message.Chat.ID)
				continue
			}
			payload := body[11477:]
			if fullTextMdFile {
				sendHtmlChunkWithText(payload, searchWord, update.Message.Chat.ID)
			} else {
				packet := pack{
					rawBytes:  &payload,
					chatID:    update.Message.Chat.ID,
					wordTitle: searchWord,
				}
				downstream <- packet
			}
		} else if update.CallbackQuery != nil {
			queryData := strings.Split(update.CallbackQuery.Data, ".")
			log.Println("Handling callback, word: ", queryData[0])
			cachedData, ok := memoryCache.read(queryData[0])
			if !ok {
				log.Println("No data in cache")
				continue
			}
			pageNum, err := strconv.Atoi(queryData[1])
			if err != nil {
				log.Printf("Error on string to int convert: %s", err)
				continue
			}
			message := tgbotapi.NewEditMessageText(update.CallbackQuery.Message.Chat.ID,
				update.CallbackQuery.Message.MessageID,
				cachedData.textList[pageNum])
			keyboard, ok := getKeyboard(getWordKeyboardData(queryData[0], &cachedData, pageNum))
			if ok {
				message.ReplyMarkup = &keyboard
			}
			sendMessage(message)
			continue
		}
	}
}
