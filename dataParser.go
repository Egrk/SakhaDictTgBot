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
	wordTitle string
	wordExplain Word
	chatID int64
	number int
	rawBytes *[]byte
}

type Word struct {
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
	wordStruct := Word{}
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
	wordModel := cachedWord{}
	for i := 0; i < num; i++ {
		if packSlice[i].chatID != 0 {
			divideToChunks(packSlice[i], &wordModel)
			continue
		}
		for {
			handledPack := <- callback
			if handledPack.number == i {
				divideToChunks(handledPack, &wordModel)
				break
			}
			packSlice[handledPack.number] = handledPack
		}
	}
	memoryCache.update(packet.wordTitle, wordModel)
	msg := tgbotapi.NewMessage(packet.chatID, wordModel.textList[0])
	keyboard, ok := getKeyboard(getWordKeyboardData(packet.wordTitle, &wordModel, 0))
	if ok {
		msg.ReplyMarkup = keyboard
	}
	if _, err := bot.Send(msg); err != nil {
		log.Fatal("error happened", err)
	}
}

func getWordKeyboardData(key string, wordModel *cachedWord, currentPos int) (string, string, string, string) {
	leftPage, rightPage := "", ""
	prevChapter, nextChapter := "", ""
	if currentPos != 0 {
		leftPage = key+"."+strconv.Itoa(currentPos - 1)
	}
	if len(wordModel.textList) - 1 != currentPos {
		rightPage = key+"."+strconv.Itoa(currentPos + 1)
	}
	if len(wordModel.chaptersNumber) > 1 {
		for idx, val := range wordModel.chaptersNumber {
			if val >= currentPos || idx == len(wordModel.chaptersNumber) - 1 {
				if currentPos == val && idx != 0 {
					prevChapter = key+"."+strconv.Itoa(wordModel.chaptersNumber[idx-1])
				} else if currentPos > val {
					prevChapter = key+"."+strconv.Itoa(val)
				}
				if len(wordModel.chaptersNumber) - 1 != idx {
					nextChapter = key+"."+strconv.Itoa(wordModel.chaptersNumber[idx+1])
				}
				break
			}
		}
	}
	return leftPage, rightPage, prevChapter, nextChapter
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

var divideToChunks = func(pack pack, cacheModel *cachedWord) { // For test purpose
	if len(pack.wordExplain.texts) > 0 {
		text := pack.wordExplain.head + "\n"
		cacheModel.chaptersNumber = append(cacheModel.chaptersNumber, len(cacheModel.textList))
		for _, wordBody := range pack.wordExplain.texts {
			if utf8.RuneCountInString(text)+utf8.RuneCountInString(wordBody+"\n") < 1024 { 
				text += wordBody + "\n"
			} else {
				cacheModel.textList = append(cacheModel.textList, text)
				text = ""
			}
		}
		if text != "" {
			cacheModel.textList = append(cacheModel.textList, text)
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
