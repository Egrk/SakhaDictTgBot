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
	helpMessage    = "Всё просто: отправляешь мне слово на якутском языке, я выдаю его значение. Знаком '+' в начале можно получить html-файл с полным пояснение слова с примерами. Пример запроса: '+Саха'"
	startMessage   = "Вас приветствует бот Большого толкового словаря якутского языка. Данные берутся из его электронной версии по ссылке: https://igi.ysn.ru/btsja/index.php. Бот некоммерческий и создан только для удобного взаимодействия со словарем"
	defaultMessage = "Такой команды не знаю :("
	htmlStartElement = "<div class = 'text'>"
	htmlEndElement = "</div>"
)

type pack struct {
	wordExplain word
	id int64
	bot *tgbotapi.BotAPI
}

type word struct {
	head  string
	texts []string
	rawData string
}

func sentenceParser(pack pack, callback chan int) {
	runeList := []rune(pack.wordExplain.rawData)
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
	if len(ans) != 0 {
		pack.wordExplain.texts = ans
		iterateAndSend(pack)
	}
	callback <- 1
}

func parseHtmlBody(text string, pack pack, downstream chan pack) (data []word) {
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
			Loop:
			for {
				switch tkn.Next() {
				case html.TextToken: 
					t := tkn.Token()
					if strings.TrimSpace(t.Data) != "" {
						vals = append(vals, t.Data)
					}
				// case html.StartTagToken:
				// 	if t := tkn.Token(); t.Data == "br" && isFullText {
				// 		if isPreviousBr {
				// 			vals = append(vals, "\n")
				// 			isPreviousBr = false
				// 		} else {
				// 			isPreviousBr = true
				// 		}
				// 	}
				case html.EndTagToken:
					t := tkn.Token()
					if t.Data == "div" {
						break Loop
					}
				}
			}
			wordStruct.head = vals[0]
			wordStruct.rawData = strings.Join(vals[1:], "")
			pack.wordExplain = wordStruct
			downstream <- pack
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

func iterateAndSend(pack pack) {
	text := pack.wordExplain.head + "\n"
	for _, wordBody := range pack.wordExplain.texts {
		if utf8.RuneCountInString(text)+utf8.RuneCountInString(wordBody+"\n") < 4096 {
			text += wordBody + "\n"
		} else {
			sendMessage(text, pack.id, pack.bot)
			text = ""
		}
	}
	if text != "" {
		sendMessage(text, pack.id, pack.bot)
	}
}

func sendHtmlChunkWithText(body string, searchWord string, id int64, bot *tgbotapi.BotAPI) {
	startIndex := strings.Index(body, htmlStartElement)
	endIndex := strings.LastIndex(body, htmlEndElement)
	if startIndex == -1 || endIndex == -1 {
		sendMessage("Слово не найдено", id, bot)
		return
	}
	bodyChunk := body[startIndex:endIndex]
	file := tgbotapi.FileReader{
		Name: searchWord + ".html",
		Reader: strings.NewReader(bodyChunk),
	}
	msg := tgbotapi.NewDocument(id, file)
	bot.Send(msg)
}

func main() {
	// config, err := loadConfig(".")
	apiKey := os.Getenv("API_KEY")
	port := os.Getenv("PORT")
	host := os.Getenv("HOST")
	if apiKey == "" || port == "" || host == "" {
		log.Fatal("API_KEY, PORT and HOST must be set")
		return
	}
	
	bot, err := tgbotapi.NewBotAPI(apiKey)
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

	updates := bot.ListenForWebhook("/" + bot.Token)
	urlAddress, _ := url.Parse(dictURL)
	log.Println("Configs setted, starting listening")
	downstream := make(chan pack)
	go balancer(downstream)
	for update := range updates {
		if update.Message == nil {
			continue
		}
		fullTextMdFile := false
		if update.Message.IsCommand() {
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
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal("error happened ", err)
		}
		resp.Body.Close()
		pack := pack{
			id: update.Message.Chat.ID,
			bot: bot,
		}
		if len(body) < 14000 {
			sendMessage("Слово не найдено", update.Message.Chat.ID, bot)
			continue
		}
		stringBodyChunk := string(body[11477:])
		if fullTextMdFile {
			sendHtmlChunkWithText(stringBodyChunk, searchWord, update.Message.Chat.ID, bot)
		} else {
			parseHtmlBody(stringBodyChunk, pack, downstream)
		}
	}
}
