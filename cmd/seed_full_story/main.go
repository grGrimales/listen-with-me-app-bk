package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"listen-with-me/backend/internal/model"
)

func main() {
	loginURL := "http://localhost:8082/api/auth/login"
	loginBody, _ := json.Marshal(map[string]string{
		"email":    "admin@listenwithme.com",
		"password": "admin1234",
	})

	resp, err := http.Post(loginURL, "application/json", bytes.NewBuffer(loginBody))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var loginRes struct {
		Token string `json:"token"`
	}
	json.NewDecoder(resp.Body).Decode(&loginRes)

	if loginRes.Token == "" {
		log.Fatal("Failed to get token")
	}

	storyReq := model.CreateFullStoryRequest{
		Title:      "The Whispering Wind",
		Level:      "A2",
		CategoryID: 1, // General
		Author:     "Elena Vance",
		CoverURL:   "https://images.unsplash.com/photo-1441974231531-c6227db76b6e?q=80&w=2560&auto=format&fit=crop",
		Paragraphs: []model.FullParagraph{
			{
				Position: 1,
				Content:  "The wind whispers secrets through the tall green trees of the valley.",
				Translations: []model.CreateTranslationRequest{
					{Language: "es", Content: "El viento susurra secretos a través de los altos árboles verdes del valle."},
				},
				Vocabulary: []model.CreateVocabularyRequest{
					{Word: "Whispers", Definition: "To speak very softly"},
					{Word: "Valley", Definition: "A low area of land between hills or mountains"},
				},
			},
			{
				Position: 2,
				Content:  "Every morning, a young girl named Maya listens to these stories.",
				Translations: []model.CreateTranslationRequest{
					{Language: "es", Content: "Cada mañana, una joven llamada Maya escucha estas historias."},
				},
				Vocabulary: []model.CreateVocabularyRequest{
					{Word: "Listens", Definition: "To give attention to sound"},
				},
			},
			{
				Position: 3,
				Content:  "She believes the forest is alive and has many things to tell her.",
				Translations: []model.CreateTranslationRequest{
					{Language: "es", Content: "Ella cree que el bosque está vivo y tiene muchas cosas que decirle."},
				},
			},
		},
		Voices: []model.CreateVoiceRequest{
			{
				Name:     "British Female",
				AudioURL: "https://www.soundhelix.com/examples/mp3/SoundHelix-Song-1.mp3", // Placeholder
				Timestamps: []model.VoiceTimestamp{
					{ParagraphID: 1, StartMs: 0, EndMs: 5000},
					{ParagraphID: 2, StartMs: 5000, EndMs: 10000},
					{ParagraphID: 3, StartMs: 10000, EndMs: 15000},
				},
			},
		},
	}

	body, _ := json.Marshal(storyReq)
	req, _ := http.NewRequest("POST", "http://localhost:8082/api/stories/full", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+loginRes.Token)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		log.Fatalf("Failed to create story: %s", string(respBody))
	}

	fmt.Println("Full story with voices created successfully!")
}
