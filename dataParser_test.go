package main

import (
	"fmt"
	"testing"
)

const (
	testDataTemplate = `<ol><hr class="hr"><div class="text"><br><li><b>%s</b> (1  том,  436  страница).<p><b><i>аат.</i></b>%s<b>Θ</b>%s</p><br></li></div>`
	bodyTemplate = "1) %sΘ %s"
)

func TestParseHtmlBody(t *testing.T) {
	testCases := [][]string{
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
			"Первый тестовый текст.", 
			"Второй тестовый текст.",
		},
		{
			"Второй заголовок", 
			"Этого предложения не должно быть. Первое предложение.", 
			"Второе предложение. Этого не должно быть.", 
			"Первое предложение.",
			"Второе предложение.",
		},
	}
	var head, text string
	iterateAndSend = func(pack pack) {
		head = pack.wordExplain.head
		text = pack.wordExplain.texts[0]
	}
	mockChan := make(chan struct{}, 1)
	for _, value := range testCases {
		testCase := fmt.Sprintf(testDataTemplate, value[0], value[1], value[2])
		mockChan <- struct{}{}
		parseHtmlBody([]byte(testCase), 1234, mockChan)
		if head == "" && text == "" {
			t.Fatalf("function iterateAndSend not called")
		}
		if head != value[0] {
			t.Errorf("wrong head (%s) got, want %s",
			head, value[0])
		}
		body := fmt.Sprintf(bodyTemplate, value[3], value[4])
		if text != body {
			t.Errorf("wrong text (%s) got, want %s",
			text, body)
		}
	}
}