package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/sashabaranov/go-openai"
)

type Item struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

type GoogleResponse struct {
	Items []Item `json:"items"`
}

// Replace with your actual key
type ChatRequest struct {
	Message string `json:"message"`
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	fmt.Print("==============================================\n")
	fmt.Print("Welcome to the Google Search Engine && AI Help\n")
	fmt.Print("==============================================\n")

	reader := bufio.NewReader(os.Stdin) // For efficient input reading

	for {
		fmt.Print("What type of help do you need? \n")
		fmt.Print("1. Search for something\n")
		fmt.Print("2. AI Help\n")
		fmt.Print("3. Exit\n")

		input, _ := reader.ReadString('\n') // Read until newline
		input = strings.TrimSpace(input)    // Remove leading/trailing whitespace

		switch input { // Use input directly
		case "1":
			search()
		case "2":
			aiHelp()
		case "3":
			fmt.Println("Exiting...")
			return // Terminate the program
		default:
			fmt.Print("Invalid Option\n")
		}
	}
}

func aiHelp() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	fmt.Print("==============================================\n")
	fmt.Print("Welcome to the AI Help\n")
	fmt.Print("==============================================\n")
	fmt.Print("Please enter your prompt\n")

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
}

func search() {
	fmt.Print("Search Query::")
	// Read the search query from the user
	reader := bufio.NewReader(os.Stdin)
	searchQuery, _ := reader.ReadString('\n')
	searchQuery = strings.Replace(searchQuery, " ", "+", -1)
	searchQuery = strings.TrimSuffix(searchQuery, "\n")

	apiURL := "https://www.googleapis.com/customsearch/v1?key=" + os.Getenv("API_KEY") + "&cx=" + os.Getenv("CX") + "&q=" + searchQuery
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
}
