package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// Google Calendar APIのサービスを取得
func getCalendarService() (*calendar.Service, error) {
	ctx := context.Background()

	// 環境変数からサービスアカウントの認証情報を取得
	credentials := os.Getenv("GOOGLE_CREDENTIALS")
	if credentials == "" {
		return nil, fmt.Errorf("GOOGLE_CREDENTIALS environment variable not set")
	}

	// 認証情報をJSONとしてパース
	creds := []byte(credentials)
	config := make(map[string]interface{})
	if err := json.Unmarshal(creds, &config); err != nil {
		return nil, fmt.Errorf("unable to parse GOOGLE_CREDENTIALS: %v", err)
	}

	// 認証情報を使ってGoogle Calendar APIクライアントを作成
	srv, err := calendar.NewService(ctx, option.WithCredentialsJSON(creds))
	if err != nil {
		return nil, fmt.Errorf("unable to create Calendar client: %v", err)
	}

	return srv, nil
}

// イベントを取得し、1日前のものをフィルタリング
func getEvents(srv *calendar.Service, calendarID string) ([]*calendar.Event, error) {
	t := time.Now().Format(time.RFC3339)
	events, err := srv.Events.List(calendarID).ShowDeleted(false).
		SingleEvents(true).TimeMin(t).OrderBy("startTime").Do()
	if err != nil {
		return nil, err
	}
	return events.Items, nil
}

// DiscordのWebhookに通知を送信
func sendDiscordNotification(webhookURL string, message string) error {
	payload := map[string]string{
		"content": message,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(webhookURL, "application/json", strings.NewReader(string(payloadBytes)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to send notification: %v", resp.Status)
	}
	return nil
}

func main() {
	// Google Calendar APIのサービスを取得
	calendarService, err := getCalendarService()
	if err != nil {
		log.Fatalf("Unable to retrieve Calendar client: %v", err)
	}

	// 環境変数からGoogle Calendar IDとDiscord Webhook URLを取得
	calendarID := os.Getenv("GOOGLE_CALENDAR_ID")
	webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")

	// タイムゾーンを固定で日本時間（Asia/Tokyo）に設定
	location := time.FixedZone("Asia/Tokyo", 9*60*60)

	// 現在の日本時間の取得
	now := time.Now().In(location)

	// 明日の開始時間（日本時間）
	tomorrowStart := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, location)
	// 明日の終了時間（日本時間）
	tomorrowEnd := tomorrowStart.Add(24 * time.Hour)

	// カレンダーのイベントを取得
	events, err := getEvents(calendarService, calendarID)
	if err != nil {
		log.Fatalf("Unable to retrieve events: %v", err)
	}

	// 明日開催されるイベントのみをフィルタリングしてログ出力
	for _, event := range events {
		// イベントの開始時間を取得（DateTimeまたはDate）
		var startTime, endTime time.Time
		var isAllDay bool

		log.Println("Event ID:", event.Id)
		log.Println("Event Summary:", event.Summary)
		log.Println("Event Description:", event.Description)
		log.Println("Event Location:", event.Location)
		log.Println("Event Start DateTime:", event.Start.DateTime)
		log.Println("Event End DateTime:", event.End.DateTime)

		if event.Start.DateTime != "" {
			// 時間が設定されているイベント
			startTime, err = time.Parse(time.RFC3339, event.Start.DateTime)
			if err != nil {
				log.Printf("Unable to parse event DateTime: %v", err)
				continue
			}
			endTime, err = time.Parse(time.RFC3339, event.End.DateTime)
			if err != nil {
				log.Printf("Unable to parse event End DateTime: %v", err)
				continue
			}
		} else if event.Start.Date != "" {
			// 終日イベントの場合
			startTime, err = time.Parse("2006-01-02", event.Start.Date)
			if err != nil {
				log.Printf("Unable to parse event Start Date: %v", err)
				continue
			}
			endTime, err = time.Parse("2006-01-02", event.End.Date)
			if err != nil {
				log.Printf("Unable to parse event End Date: %v", err)
				continue
			}
			isAllDay = true
		} else {
			log.Printf("No valid start time found for event: %s", event.Summary)
			continue
		}

		// 開始時間と終了時間を日本時間に変換
		startTime = startTime.In(location)
		endTime = endTime.In(location)

		// イベントが明日開催されるか確認
		if startTime.After(tomorrowStart) && startTime.Before(tomorrowEnd) {
			// イベントの詳細を info レベルでログ出力
			log.Println("イベント名:", event.Summary)
			log.Println("説明:", event.Description)
			log.Println("場所:", event.Location)

			if isAllDay {
				log.Println("終日イベント:", event.Summary)
			} else {
				log.Println("開始時間:", startTime.Format("2006-01-02 15:04"))
				log.Println("終了時間:", endTime.Format("2006-01-02 15:04"))
			}

			// 通知メッセージを作成（ログ出力に合わせた形式）
			var message string
			if isAllDay {
				message = fmt.Sprintf("@here\nイベント名: %s\n場所: %s\n終日イベント", event.Summary, event.Location)
			} else {
				message = fmt.Sprintf("@here\nイベント名: %s\n場所: %s\n開始時間: %s\n終了時間: %s",
					event.Summary, event.Location, startTime.Format("2006-01-02 15:04"), endTime.Format("2006-01-02 15:04"))
			}

			// Discordに通知を送信
			err := sendDiscordNotification(webhookURL, message)
			if err != nil {
				log.Printf("Error sending Discord notification: %v", err)
			} else {
				log.Println("Notification sent for event:", event.Summary)
			}
		}
	}
}
