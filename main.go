package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/joho/godotenv"
	"github.com/ktr0731/go-fuzzyfinder"
	"github.com/pkg/browser"
	"github.com/sashabaranov/go-openai"
)

type Mode int

const (
	InputMode Mode = iota
	SearchMode
	AIMode
)

var currentMode Mode = InputMode

type Item struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

type GoogleResponse struct {
	Items []Item `json:"items"`
}

type ChatRequest struct {
	Message string `json:"message"`
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	fmt.Print("===================================================\n")
	fmt.Print("Welcome to Trm Search \n")
	fmt.Print("Type :s to search, :ai to chat with AI, :qa to quit\n")
	fmt.Print("===================================================\n")

	for {
		switch currentMode {
		case InputMode:
			handleInputMode()
		case SearchMode:
			handleSearchMode()
		case AIMode:
			handleAIMode()
		}
	}

}

func handleInputMode() {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == ":s" {
			currentMode = SearchMode
			return
		} else if input == ":ai" {
			currentMode = AIMode
			return
		} else if input == ":qa" {
			fmt.Println("Exiting...")
			os.Exit(0)
		} else {
			handleInputCommand(input)
		}
	}
}

func handleInputCommand(command string) {
	cmd := exec.Command("bash", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Println("Error executing command:", err)
	}
}

func handleSearchMode() {
	fmt.Print("Search Query::")

	reader := bufio.NewReader(os.Stdin)
	searchQuery, _ := reader.ReadString('\n')

	searchQuery = strings.Replace(searchQuery, " ", "+", -1)
	searchQuery = strings.TrimSuffix(searchQuery, "\n")

	apiURL := os.Getenv("CUSTOM_SEARCH_API_ENDPOINT") + os.Getenv("GOOGLE_API_KEY") + "&cx=" + os.Getenv("CX") + "&q=" + searchQuery
	req, _ := http.NewRequest("GET", apiURL, nil)

	res, err := http.DefaultClient.Do(req)

	if err != nil {
		log.Fatal(err)
	}

	defer res.Body.Close()
	var searchResponse GoogleResponse

	json.NewDecoder(res.Body).Decode(&searchResponse)

	for i, item := range searchResponse.Items {
		fmt.Printf("%d: %s - \n %s \n %s- \n", i+1, item.Title, item.Link, item.Snippet)
		if i >= 9 {
			break
		}
	}

	idx, err := fuzzyfinder.FindMulti(
		searchResponse.Items,
		func(i int) string {
			return searchResponse.Items[i].Title
		},
		fuzzyfinder.WithPreviewWindow(func(i, w, h int) string {
			if i == -1 {
				return ""
			}
			return fmt.Sprintf("Search Result: %s (%s)\nresult: %s",
				searchResponse.Items[i].Title,
				searchResponse.Items[i].Snippet,
				searchResponse.Items[i].Link)
		}))

	if err != nil {
		log.Fatal(err)
	}

	if idx != nil {
		err := browser.OpenURL(searchResponse.Items[idx[0]].Link)
		if err != nil {
			fmt.Println("Error opening browser:", err)
		}
	}

	handleInputMode()
}

func handleAIMode() {
	err := godotenv.Load()

	if err != nil {
		log.Fatal("Error loading .env file")
	}

	fmt.Print("Please enter your prompt:: ")

	reader := bufio.NewReader(os.Stdin)
	aiPrompt, _ := reader.ReadString('\n')

	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: aiPrompt,
				},
			},
		},
	)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("ChatCompletion response: %v\n", resp.Choices[0].Message.Content)

	handleInputMode()
}
