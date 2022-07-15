package jia

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-redis/redis"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

var (
	jiaConfig   *Config
	redisClient *redis.Client
	slackClient *slack.Client
)

func StartServer(config *Config) {
	jiaConfig = config
	// Set up Redis connection
	options, err := redis.ParseURL(config.RedisURL)
	if err != nil {
		panic(err)
	}
	redisClient = redis.NewClient(options)

	// Initialize default values
	redisClient.SetNX("last_sender_id", "", 0)
	redisClient.SetNX("last_valid_number", 0, 0)
	redisClient.SetNX("last_count_at", 0, 0)

	// Initialize Slack app
	slackClient = slack.New(config.BotToken)

	// Start receiving events
	http.HandleFunc("/slack/events", handleSlackEvents)
	http.HandleFunc("/slack/leaderboard", HandleLeaderboardSlashCommand)
	http.HandleFunc("/slack/eventsCommand", HandleEventsSlashCommand)
	http.HandleFunc("/api/currentNumber", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")

		number, err := redisClient.Get("last_valid_number").Int()
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}

		response, _ := json.Marshal(struct {
			CurrentNumber int `json:"number"`
		}{
			CurrentNumber: number,
		})

		w.Header().Add("Content-Type", "application/json")
		w.Write(response)
	})
	http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", config.Port), nil)
}

func handleSlackEvents(w http.ResponseWriter, r *http.Request) {
	// Verify the payload was sent by Slack.
	buf := new(bytes.Buffer)
	buf.ReadFrom(r.Body)
	body := buf.String()
	apiEvent, err := slackevents.ParseEvent(json.RawMessage(body),
		slackevents.OptionVerifyToken(
			&slackevents.TokenComparator{VerificationToken: jiaConfig.VerificationToken}))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Handle the event that came through
	switch apiEvent.Type {
	case slackevents.URLVerification:
		var r *slackevents.ChallengeResponse
		err := json.Unmarshal([]byte(body), &r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}

		w.Header().Set("Content-Type", "text")
		w.Write([]byte(r.Challenge))
		break
	case slackevents.CallbackEvent:
		HandleInnerEvent(slackClient, &apiEvent.InnerEvent)
		break
	}
}
