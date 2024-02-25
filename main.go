package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Item struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

type GoogleResponse struct {
	Items []Item `json:"items"`
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	fmt.Println("Search Query::")
	// Read the search query from the user
	reader := bufio.NewReader(os.Stdin)
	searchQuery, _ := reader.ReadString('\n')
	// format the search query
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
