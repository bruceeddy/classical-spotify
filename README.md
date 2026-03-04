# Classical - Spotify Track Search Application

A command-line application that searches the Spotify API for tracks matching a given query. Perfect for discovering classical music and other tracks on Spotify.

## Features

- **Command-line search**: Search for tracks directly from your terminal
- **Spotify API integration**: Leverages the official Spotify Web API
- **Detailed track information**: Get track names, artists, albums, and Spotify URLs
- **Multiple results**: Returns up to 20 matching tracks per search
- **Easy to use**: Simple, intuitive command-line interface

## Prerequisites

Before you can use this application, you'll need:

1. **Go 1.16 or later** - [Download Go](https://golang.org/dl/)
2. **Spotify Developer Account** - [Register here](https://developer.spotify.com)
3. **Spotify API Credentials** - Client ID and Client Secret from your Spotify app

## Setup Instructions

### Step 1: Register a Spotify Application

1. Go to https://developer.spotify.com/dashboard
2. Log in with your Spotify account (create one if needed)
3. Accept the terms and create a new application
4. You'll receive a **Client ID** and **Client Secret**

### Step 2: Configure the Application

Copy the example environment file and fill in your credentials:

```bash
cp .env.example .env
```

Edit `.env` and replace the placeholder values:

```dotenv
SPOTIFY_CLIENT_ID=your_actual_client_id
SPOTIFY_CLIENT_SECRET=your_actual_client_secret
```

Alternatively, export the variables directly in your shell:

```bash
export SPOTIFY_CLIENT_ID="your_actual_client_id"
export SPOTIFY_CLIENT_SECRET="your_actual_client_secret"
```

> **Note:** Never commit your `.env` file to version control. It is already listed in `.gitignore`.

### Step 3: Build the Application

```bash
go mod download
go build -o classical
```

This creates an executable named `classical` in your current directory.

## Usage

### Basic Syntax

```bash
./classical "<search query>"
```

### Examples

Search for a specific composer:
```bash
./classical "Mozart"
```

Search for a specific symphony:
```bash
./classical "Beethoven Symphony No. 9"
```

Search for multiple words (use quotes for spaces):
```bash
./classical "Bach Cello Suites"
```

### Output

The application displays:
1. Number of tracks found
2. For each track:
   - Track name
   - Artist(s)
   - Album name
   - Direct Spotify link

Example output:
```
Searching Spotify for: "Mozart Symphony No. 40"

Found 15 track(s):

1. Symphony No. 40 in G minor K.550
   Artists: Wolfgang Amadeus Mozart, Berlin Philharmonic Orchestra
   Album: The 41 Symphonies
   URL: https://open.spotify.com/track/3qm4B4dczCow5p15...

2. Symphony No. 40 in G minor, K.550: I. Molto Allegro
   Artists: Wolfgang Amadeus Mozart, Academy of St Martin in the Fields
   Album: Complete Mozart Symphonies
   URL: https://open.spotify.com/track/7xQbMyqXhzGYu...

...
```

## How It Works

1. **Authentication**: Uses Spotify's Client Credentials OAuth flow to obtain an access token
2. **Search**: Sends your search query to the Spotify Web API
3. **Results**: Parses the JSON response and displays the top 20 matching tracks
4. **Links**: Provides direct Spotify URLs for easy access

## Error Handling

The application handles common errors gracefully:

- **Missing search query**: Displays usage instructions
- **Authentication failures**: Reports token retrieval errors
- **Search failures**: Shows API errors
- **No results**: Notifies you if no tracks match your query

Example:
```bash
$ ./classical
Usage: classical <search query>
Example: classical "Mozart Symphony No. 40"
```

## API Limits

- **Results per search**: Up to 20 tracks
- **Rate limiting**: Spotify applies rate limits; if exceeded, you'll receive an error message

## Troubleshooting

### "Error getting Spotify token"
- Verify your Client ID and Client Secret are correct
- Check that your Spotify app is active and not revoked
- Ensure you have an internet connection

### "Error searching Spotify"
- Check your internet connection
- Try a simpler search query
- Verify you're not hitting Spotify's rate limits (wait a few minutes before trying again)

### No results found
- Try a more general search term
- Check the spelling of artist/track names
- Try just the artist name without specific track details

## Technical Details

- **Language**: Go (Golang)
- **API**: Spotify Web API v1
- **Authentication**: OAuth 2.0 Client Credentials flow
- **HTTP Client**: Go's standard `net/http` package

## Building from Source

```bash
# Clone or navigate to the project directory
cd classical

# Download dependencies (if any)
go mod download

# Build the executable
go build -o classical

# Run the application
./classical "search query"
```

## License

This project is provided as-is for educational and personal use.

## Resources

- [Spotify Web API Documentation](https://developer.spotify.com/documentation/web-api)
- [Go Documentation](https://golang.org/doc/)
- [Spotify Developer Dashboard](https://developer.spotify.com/dashboard)

## Support

For issues with the Spotify API, visit the [Spotify Developer Community](https://developer.spotify.com/community).

For Go-related questions, check out the [Go FAQ](https://golang.org/doc/faq).
