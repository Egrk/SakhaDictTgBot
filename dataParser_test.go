package main

import (
	"fmt"
	"os"
	"testing"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	testDataTemplate = `<ol><hr class="hr"><div class="text"><br><li><b>%s</b> (1  том,  436  страница).<p><b><i>аат.</i></b>%s<b>Θ</b>%s</p><br></li></div>`
	bodyTemplate = "%s\n1) %sΘ %s\n"
)

var testCases = [][]string{
	/*
	{
		Head, 
		First test sentece, 
		Second test sentence, 
		Want first sentence, 
		Want second sentence
	}
	*/
	{
		"Заголовок", 
		"Первый тестовый текст.", 
		"Второй тестовый текст.", 
		"Первый тестовый текст", 
		"Второй тестовый текст",
	},
	{
		"Второй заголовок", 
		"Этого предложения не должно быть. Первое предложение.", 
		"Второе предложение. Этого не должно быть.", 
		"Первое предложение",
		"Второе предложение",
	},
}

func TestParseHtmlBody(t *testing.T) {
	var text string
	sendMessage = func(message tgbotapi.Chattable) {
		text = message.(tgbotapi.MessageConfig).Text
	}
	memoryCache = newCache()
	mockChan := make(chan struct{}, 1)
	for _, value := range testCases {
		testCase := fmt.Sprintf(testDataTemplate, value[0], value[1], value[2])
		byteTestCase := []byte(testCase)
		packet := pack{
			rawBytes: &byteTestCase,
			chatID: 1234,
		}
		mockChan <- struct{}{}
		parseHtmlBody(packet, mockChan)
		if text == "" {
			t.Fatalf("function divideToChunks not called")
		}
		body := fmt.Sprintf(bodyTemplate, value[0], value[3], value[4])
		if text != body {
			t.Errorf("wrong text (%s) got, want %s",
			text, body)
		}
	}
}

func BenchmarkParseHtmlBody(b *testing.B) {
	memoryCache = newCache()
	sendMessage = func(message tgbotapi.Chattable) {
	}
	for i := 0; i < b.N; i++ {
		mockChan := make(chan struct{}, 1)
		data, err := os.ReadFile("./TestBenchData/BenchData.htm")
		if err != nil {
			b.Fatal("Error on read file")
		}
		byteTestCase := data[11477:]
		packet := pack{
			rawBytes: &byteTestCase,
			chatID: 1234,
		}
		mockChan <- struct{}{}
		parseHtmlBody(packet, mockChan)
	}
}