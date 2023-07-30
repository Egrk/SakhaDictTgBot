package main

import (
	"bytes"
	"container/list"
	"strconv"
	"unicode"
	"unicode/utf8"
	"golang.org/x/net/html"
	"strings"
	"log"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var counter = 0
var callback = make(chan pack)
var queue = list.New()

type pack struct {
	wordExplain word
	chatID int64
	number int
}

type word struct {
	head  string
	texts []string
	rawData string
}

type searchSettings struct {
	raw []byte
	chatID int64
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
	}
	callback <- pack
}

func parseHtmlBody(text []byte, chatID int64, done <-chan struct{}) {
	defer func ()  {
		<-done
	}()
	tkn := html.NewTokenizer(bytes.NewReader(text))
	wordStruct := word{}
	var vals []string
	callback := make(chan pack)
	num := 0
	packet := pack{
		chatID: chatID,
	}
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
		if packArray[i].chatID != 0 {
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

func sendMessage(text string, id int64) {
	msg := tgbotapi.NewMessage(id, text)
	if _, err := bot.Send(msg); err != nil {
		log.Fatal("error happened", err)
	}
}

func iterateAndSend(pack pack) {
	if len(pack.wordExplain.texts) > 0 {
		text := pack.wordExplain.head + "\n"
		for _, wordBody := range pack.wordExplain.texts {
			if utf8.RuneCountInString(text)+utf8.RuneCountInString(wordBody+"\n") < 4096 {
				text += wordBody + "\n"
			} else {
				sendMessage(text, pack.chatID)
				text = ""
			}
		}
		if text != "" {
			sendMessage(text, pack.chatID)
		}
	}
}

func sendHtmlChunkWithText(body []byte, searchWord string, id int64) {
	startIndex := bytes.Index(body, []byte(htmlStartElement))
	endIndex := bytes.LastIndex(body, []byte(htmlEndElement))
	if startIndex == -1 || endIndex == -1 {
		sendMessage("Слово не найдено", id)
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

func balancer(upstream <-chan searchSettings) {
	var semafore = make(chan struct{}, 15)
	for elem := range upstream {
		semafore <-struct{}{}
		go parseHtmlBody(elem.raw, elem.chatID, semafore)
	}
}