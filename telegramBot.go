package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/net/html"
)

const (
	dictURL        = "https://igi.ysn.ru/btsja/index.php?data1=&talww=1"
	helpMessage    = "Всё просто: отправляешь мне слово на якутском языке, я выдаю его значение"
	startMessage   = "Вас приветствует бот Большого толкового словаря якусткого языка. Данные берутся из его электронной версии по ссылке: https://igi.ysn.ru/btsja/index.php. Бот некоммерческий и создан только для удобного взаимодействия со словарем"
	defaultMessage = "Такой команды не знаю :("
)

type word struct {
	head  string
	texts []string
}

func sentenceParser(rawData string) []string {
	runeList := []rune(rawData)
	var lastSentence []string
	check := false
	lenOfList := len(runeList)
	var ans []string
	indx := 0
	number := 1
	for indx < lenOfList {
		if runeList[indx] == 'Θ' {
			idnxStep, nextSentence := firstSentenceParse(runeList[indx+1:])
			indx += idnxStep
			firstSentence := strings.Join(lastSentence, "")
			ans = append(ans, strconv.Itoa(number)+") "+firstSentence+"Θ "+nextSentence)
			number++
		}
		if check {
			if indx+2 < lenOfList {
				if runeList[indx] == '.' && (unicode.IsUpper(runeList[indx+2]) || runeList[indx+2] == ' ') {
					check = false
				}
			}
			lastSentence = append(lastSentence, string(runeList[indx]))
		} else if unicode.IsUpper(runeList[indx]) {
			lastSentence = nil
			lastSentence = append(lastSentence, string(runeList[indx]))
			check = true
		}
		indx++
	}
	return ans
}

func parse(text string) (data []word) {
	tkn := html.NewTokenizer(strings.NewReader(text))
	wordStruct := word{}
	var vals []string
	var listOfWordStruct []word
	for {
		tt := tkn.Next()
		if tt == html.ErrorToken {
			return listOfWordStruct
		}
		if tt != html.StartTagToken {
			continue
		}
		t := tkn.Token()
		if t.Data == "div" && t.Attr[0].Key == "class" && t.Attr[0].Val == "text" {
			for {
				tt = tkn.Next()
				if tt == html.TextToken {
					t := tkn.Token()
					if strings.TrimSpace(t.Data) != "" {
						vals = append(vals, t.Data)
					}
				} else if tt == html.EndTagToken {
					t := tkn.Token()
					if t.Data == "div" {
						break
					}
				}
			}
			wordStruct.head = vals[0]
			rawData := strings.Join(vals[1:], "")
			wordStruct.texts = sentenceParser(rawData)
			if len(wordStruct.texts) != 0 {
				listOfWordStruct = append(listOfWordStruct, wordStruct)
			}
		}
		vals = nil
	}
}

func firstSentenceParse(text []rune) (int, string) {
	var ans []string
	idx := 0
	for i := 0; i < len(text); i++ {
		ans = append(ans, string(text[i]))
		if text[i] == '.' {
			if i+2 < len(text) {
				if unicode.IsUpper(text[i+2]) || text[i+2] == ' ' {
					idx = i
					break
				}
			}
		}
	}
	return idx, strings.Join(ans, "")
}

func sendMessage(text string, id int64, bot *tgbotapi.BotAPI) {
	msg := tgbotapi.NewMessage(id, text)
	if _, err := bot.Send(msg); err != nil {
		log.Fatal("error happened", err)
	}
}

func iterateAndSend(wordStructList []word, id int64, bot *tgbotapi.BotAPI) {
	for _, m := range wordStructList {
		text := m.head + "\n"
		for _, wordBody := range m.texts {
			if utf8.RuneCountInString(text)+utf8.RuneCountInString(wordBody+"\n") < 4096 {
				text += wordBody + "\n"
			} else {
				sendMessage(text, id, bot)
				text = ""
			}
		}
		if text != "" {
			sendMessage(text, id, bot)
		}
	}
}

func main() {
	// config, err := loadConfig(".")
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY must be set")
	}
	bot, err := tgbotapi.NewBotAPI(apiKey)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)
	urlAddress, _ := url.Parse(dictURL)
	for update := range updates {
		if update.Message == nil {
			continue
		}
		if update.Message.IsCommand() {
			switch update.Message.Command() {
			case "help":
				sendMessage(helpMessage, update.Message.Chat.ID, bot)
			case "start":
				sendMessage(startMessage, update.Message.Chat.ID, bot)
			default:
				sendMessage(defaultMessage, update.Message.Chat.ID, bot)
			}
			continue
		}
		searchWord := update.Message.Text
		query := urlAddress.Query()
		query.Set("data1", searchWord)
		urlAddress.RawQuery = query.Encode()
		resp, err := http.Get(fmt.Sprint(urlAddress, searchWord))
		if err != nil {
			log.Fatal("error happened ", err)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal("error happened ", err)
		}
		resp.Body.Close()
		log.Println(len(body))
		data := parse(string(body[11477:]))
		if len(data) == 0 {
			sendMessage("Слово не найдено", update.Message.Chat.ID, bot)
			continue
		}
		iterateAndSend(data, update.Message.Chat.ID, bot)
	}
}
