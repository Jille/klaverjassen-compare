package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"

	"github.com/Jille/convreq"
	"github.com/Jille/convreq/respond"
	"github.com/mxk/go-sqlite/sqlite3"
)

var (
	databasePath = flag.String("database_path", "data.db", "Path to sqlite3 database")
	port         = flag.Int("port", 8099, "Port number to listen on")
)

func main() {
	flag.Parse()
	http.Handle("/compare/", convreq.Wrap(gamePage))
	srv := &http.Server{
		Addr: fmt.Sprintf(":%d", *port),
	}
	log.Fatal(srv.ListenAndServe())
}

type gameResult struct {
	Players        [4]string
	StartingPlayer int
	Trump          string
	Scores         [2]int
	Glory          [2]int
	Rounds         [8][4][2]string
	RoundWinners   [8]int
	RoundGlory     [8]int
}

func (r gameResult) Renderable(allGames []gameResult) renderableGame {
	playerOccurrences := map[string]int{}
	for _, og := range allGames {
		for _, p := range og.Players {
			playerOccurrences[p]++
		}
	}
	var uniquePlayers []string
	for _, p := range r.Players {
		if playerOccurrences[p] == 1 {
			uniquePlayers = append(uniquePlayers, p)
		}
	}
	ret := renderableGame{
		Players:                    r.Players,
		UniquePlayers:              uniquePlayers,
		StartingPlayer:             r.Players[r.StartingPlayer],
		Trump:                      suit(r.Trump),
		PlayingTeam:                [2]string{r.Players[r.StartingPlayer], r.Players[(r.StartingPlayer+2)%4]},
		OpposingTeam:               [2]string{r.Players[(r.StartingPlayer+1)%4], r.Players[(r.StartingPlayer+3)%4]},
		PlayingTeamScore:           r.Scores[0],
		OpposingTeamScore:          r.Scores[1],
		PlayingTeamGlory:           r.Glory[0],
		OpposingTeamGlory:          r.Glory[1],
		PlayingTeamScoreExclGlory:  r.Scores[0] - r.Glory[0],
		OpposingTeamScoreExclGlory: r.Scores[1] - r.Glory[1],
	}
	for i := 0; 8 > i; i++ {
		sp := r.StartingPlayer
		if i > 0 {
			sp = r.RoundWinners[i-1]
		}
		ret.Rounds[i] = round{
			StartingPlayer: r.Players[sp],
			Glory:          r.RoundGlory[i],
		}
		for j := 0; 4 > j; j++ {
			choices := map[string]int{}
			for _, og := range allGames {
				ogsp := og.StartingPlayer
				if i > 0 {
					ogsp = og.RoundWinners[i-1]
				}
				choices[og.Rounds[i][(j+sp-ogsp+4)%4][0]+og.Rounds[i][(j+sp-ogsp+4)%4][1]]++
			}
			ret.Rounds[i].Cards[(j+sp)%4] = playedCard{
				Value:     cardValue(r.Rounds[i][j][0]),
				Suit:      suit(r.Rounds[i][j][1]),
				Winner:    r.RoundWinners[i] == (j+sp)%4,
				Different: len(choices) != 1,
			}
		}
	}
	return ret
}

type suit string

func (s suit) Unicode() template.HTML {
	switch s {
	case "CLUBS":
		return "\u2663"
	case "SPADES":
		return "\u2660"
	case "DIAMONDS":
		return "<span style='color: red'>\u2666</span>"
	case "HEARTS":
		return "<span style='color: red'>\u2665</span>"
	default:
		return template.HTML(template.HTMLEscapeString(string(s)))
	}
}

type cardValue string

func (c cardValue) Unicode() string {
	switch c {
	case "SEVEN":
		return "7"
	case "EIGHT":
		return "8"
	case "NINE":
		return "9"
	case "TEN":
		return "10"
	case "JACK":
		return "J"
	case "QUEEN":
		return "Q"
	case "KING":
		return "K"
	case "ACE":
		return "A"
	default:
		return string(c)
	}
}

type playedCard struct {
	Suit      suit
	Value     cardValue
	Winner    bool
	Different bool
}

type round struct {
	StartingPlayer string
	Cards          [4]playedCard
	Glory          int
}

type renderableGame struct {
	UniquePlayers              []string
	Players                    [4]string
	StartingPlayer             string
	Trump                      suit
	PlayingTeam                [2]string
	OpposingTeam               [2]string
	PlayingTeamScore           int
	OpposingTeamScore          int
	PlayingTeamScoreExclGlory  int
	OpposingTeamScoreExclGlory int
	PlayingTeamGlory           int
	OpposingTeamGlory          int
	Rounds                     [8]round
}

type gamePageGet struct {
	Seed string
}

func gamePage(ctx context.Context, r *http.Request, get gamePageGet) convreq.HttpResponse {
	db, err := sqlite3.Open(*databasePath)
	if err != nil {
		return respond.Error(err)
	}
	defer db.Close()
	var games []gameResult
	var s *sqlite3.Stmt
	for s, err = db.Query("SELECT result FROM KlaverjasResults WHERE seed=?", get.Seed); err == nil; err = s.Next() {
		var d []byte
		s.Scan(&d)
		if err != nil {
			return respond.Error(err)
		}
		var g gameResult
		if err := json.Unmarshal(d, &g); err != nil {
			return respond.Error(err)
		}
		games = append(games, g)
	}
	if err != nil && err != io.EOF {
		return respond.Error(err)
	}
	switch len(games) {
	case 0:
		return respond.NotFound("No games found with this seed")
	case 1:
		return respond.NotFound("Only 1 game found with this seed, not enough to compare")
	}
	t, err := template.ParseFiles("compare.html")
	if err != nil {
		return respond.Error(err)
	}

	renderableGames := make([]renderableGame, len(games))
	for i, g := range games {
		renderableGames[i] = g.Renderable(games)
	}

	return respond.RenderTemplate(t, renderableGames)
}
