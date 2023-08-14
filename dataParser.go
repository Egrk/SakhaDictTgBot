package main

import (
	"bytes"
	"strconv"
	"unicode"
	"unicode/utf8"
	"golang.org/x/net/html"
	"strings"
	"log"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type pack struct {
	wordExplain word
	chatID int64
	number int
	rawBytes *[]byte
}

type word struct {
	head  string
	texts []string
	rawData string
}

func balancer(upstream <-chan pack) {
	var semafore = make(chan struct{}, 15)
	for elem := range upstream {
		semafore <-struct{}{}
		go parseHtmlBody(elem, semafore)
	}
}

func parseHtmlBody(packet pack, done <-chan struct{}) {
	defer func() {
		<-done
	}()
	tokenizer := html.NewTokenizer(bytes.NewReader(*packet.rawBytes))
	wordStruct := word{}
	var rawData []string
	callback := make(chan pack)
	num := 0
	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			break
		}
		if tokenType != html.StartTagToken {
			continue
		}
		token := tokenizer.Token()
		if token.Data == "div" && token.Attr[0].Key == "class" && token.Attr[0].Val == "text" {
			Loop:
			for {
				switch tokenizer.Next() {
				case html.TextToken: 
					token := tokenizer.Token()
					if strings.TrimSpace(token.Data) != "" {
						rawData = append(rawData, token.Data)
					}
				case html.EndTagToken:
					t := tokenizer.Token()
					if t.Data == "div" {
						break Loop
					}
				}
			}
			wordStruct.head = rawData[0]
			wordStruct.rawData = strings.Join(rawData[1:], "")
			packet.wordExplain = wordStruct
			packet.number = num
			num++
			go sentenceParser(packet, callback)
		}
		rawData = nil
	}
	packSlice := make([]pack, num)
	for i := 0; i < num; i++ {
		if packSlice[i].chatID != 0 {
			iterateAndSend(packSlice[i])
			continue
		}
		for {
			handledPack := <- callback
			if handledPack.number == i {
				iterateAndSend(handledPack)
				break
			}
			packSlice[handledPack.number] = handledPack
		}
	}
}

func sentenceParser(pack pack, callback chan pack) {
	runeList := []rune(pack.wordExplain.rawData)
	isSentenceEnd := false
	lenOfList := len(runeList)
	var text []string
	sentenceStartIdx := 0
	sentenceEndIdx := 0
	indx := 0
	number := 1
	for indx < lenOfList {
		if runeList[indx] == 'Θ' {
			indxStep, nextSentence := nextSentenceParse(runeList[indx+1:])
			indx += indxStep
			firstSentence := runeList[sentenceStartIdx:sentenceEndIdx]
			text = append(text, strconv.Itoa(number)+") "+string(firstSentence)+"Θ "+nextSentence)
			number++
		}
		if isSentenceEnd {
			if indx+2 < lenOfList {
				if runeList[indx] == '.' && (unicode.IsUpper(runeList[indx+2]) || runeList[indx+2] == ' ') {
					isSentenceEnd = false
				}
			} 
			sentenceEndIdx = indx
		} else if unicode.IsUpper(runeList[indx]) {
			sentenceStartIdx = indx
			isSentenceEnd = true
		}
		indx++
	}
	if len(text) != 0 {
		pack.wordExplain.texts = text
	}
	callback <- pack
}

func nextSentenceParse(text []rune) (int, string) {
	sentenceEndIdx := 0
	idx := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '.' {
			if i+2 < len(text) {
				if unicode.IsUpper(text[i+2]) || text[i+2] == ' ' {
					sentenceEndIdx = i
					break
				}
			}
			sentenceEndIdx = i
		}
	}
	return idx, string(text[:sentenceEndIdx])
}

func sendMessage(text string, id int64) {
	msg := tgbotapi.NewMessage(id, text)
	if _, err := bot.Send(msg); err != nil {
		log.Fatal("error happened", err)
	}
}

var iterateAndSend = func(pack pack) { // For test purpose
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
