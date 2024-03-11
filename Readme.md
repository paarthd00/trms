# TRM Search

This simple command-line interface (CLI) program written in Go interacts with Google's search engine and Open AI.

## Features

- Help fetches top 10 Google Search Results.

  ![alt text](images/image.png)

- Fuzzy, find the results, and open the link in the browser.

  ![alt text](images/image-1.png)

- AI help
  `:ai` to enter AI help mode where you can ask for AI help.

- Normal Mode
  If not in search or AI mode, then by default the program is in normal mode and can be used as bash terminal.

  ![alt text](images/image4.png)

## Prerequisites

- Go programming language installed on your machine.
- A valid Google Search API key and Search Engine ID (CX).
- Open AI API key

## Setup

1. Clone this repository to your local machine.
2. Navigate to the directory containing the Go files.

## Configuration

Before running the program, you need to add your Google API Search Key and Search Engine ID (CX) to the program:

Use your keys to create a new `.env` file based on the `.env.template`.

[OpenAI API](https://platform.openai.com/api-keys)

you can find `CX` and `GOOGLE_API_KEY` from
[Google Custom Search](https://developers.google.com/custom-search/v1/overview#search_engine_id)

## Installation

To install the program, run the following commands in your terminal:
`./install.sh`

This command will download the trms cli globally

## Usage

To run the program, use the following command:


```
trms
```


## License

This project is open source and available under the [MIT License](LICENSE).
