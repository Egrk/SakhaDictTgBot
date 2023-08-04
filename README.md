# Sakha Dictionary Telegram Bot

Sakha Dictionary Telegram Bot is a bot which give a brief explanation of Sakha language word. Bot take only word, not sentence.
Telegram: @SakhaDictBot

## Launching

Bot was made for Heroku, so it have Heroku optimization. For local run you should create "app.env" file and write there "API_KEY=" and paste key of telegram bot, after that run it with "-dev" key

```cmd
go run . -dev
```

To run in Heroku deploy it and add "API_KEY" and "HOST" config variables in settings. "HOST" key is URL address of your bot

## Usage

Simply send word and bot give you brief explanation. Also you can send word with "+" sign at the beginning of a word
```
+Саха
```
which give you a html file with full explanation and examples
