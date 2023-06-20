package main

import (
	"bytes"
	"flag"
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
	startMessage   = "Вас приветствует бот Большого толкового словаря якутского языка. Данные берутся из его электронной версии по ссылке: https://igi.ysn.ru/btsja/index.php. Бот некоммерческий и создан только для удобного взаимодействия со словарем. Небольшая инструкция: /help"
	defaultMessage = "Такой команды не знаю :("
	serverErrorMessage = "Ошибка сервера, попробуйте попытку позже"
	htmlStartElement = "<div class = 'text'>"
	htmlEndElement = "</div>"
)

type pack struct {
	wordExplain word
	messageId int64
	bot *tgbotapi.BotAPI
	number int
}

type word struct {
	head  string
	texts []string
	rawData string
}

func sentenceParser(pack pack, callback chan pack) {
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
		// iterateAndSend(pack)
	}
	callback <- pack
}

func parseHtmlBody(text []byte, packet pack) {
	tkn := html.NewTokenizer(bytes.NewReader(text))
	wordStruct := word{}
	var vals []string
	callback := make(chan pack)
	num := 0
	for {
		tt := tkn.Next()
		if tt == html.ErrorToken {
			break
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
			packet.wordExplain = wordStruct
			packet.number = num
			num++
			go sentenceParser(packet, callback)
		}
		vals = nil
	}
	packArray := make([]pack, num)
	for i := 0; i < num; i++ {
		if packArray[i].messageId != 0 {
			iterateAndSend(packArray[i])
			continue
		}
		for {
			handledPack := <- callback
			if handledPack.number == i {
				iterateAndSend(handledPack)
				break
			}
			packArray[handledPack.number] = handledPack
		}
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
			sendMessage(text, pack.messageId, pack.bot)
			text = ""
		}
	}
	if text != "" {
		sendMessage(text, pack.messageId, pack.bot)
	}
}

func sendHtmlChunkWithText(body []byte, searchWord string, id int64, bot *tgbotapi.BotAPI) {
	startIndex := bytes.Index(body, []byte(htmlStartElement))
	endIndex := bytes.LastIndex(body, []byte(htmlEndElement))
	if startIndex == -1 || endIndex == -1 {
		sendMessage("Слово не найдено", id, bot)
		return
	}
	bodyChunk := body[startIndex:endIndex]
	file := tgbotapi.FileReader{
		Name: searchWord + ".html",
		Reader: bytes.NewReader(bodyChunk),
	}
	msg := tgbotapi.NewDocument(id, file)
	bot.Send(msg)
}

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
