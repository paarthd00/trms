# Google Search CLI

This is a simple command-line interface (CLI) program written in Go that uses the Google Search API to fetch and display the top 10 search results.

## Features

- Fetches top 10 search results from Google Search.
- Displays results in the terminal.

## Prerequisites

- Go programming language installed on your machine.
- A valid Google Search API key and Search Engine ID (CX).

## Setup

1. Clone this repository to your local machine.
2. Navigate to the directory containing the Go files.

## Configuration

Before running the program, you need to add your Google API Search Key and Search Engine ID (CX) to the program:

Use your keys to create a new `.env` file based on the `.env.template`.

## Installation

To install the program, run the following commands in your terminal:

```bash
go mod download
go build -o main
```

This will download the necessary Go modules and build your program into an executable file named `main`.

## Usage

To run the program, use the following command:

```bash
./main
```

You will be prompted to enter a search query. After entering your query, press `Enter` to view the top 10 Google search results in your terminal.

## License

This project is open source and available under the [MIT License](LICENSE).
