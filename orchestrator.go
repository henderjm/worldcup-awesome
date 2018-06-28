package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

type MatchSet []Match

func (m MatchSet) Find(fifaID string) (Match, error) {
	for _, match := range m {
		if fifaID == match.ID {
			return match, nil
		}
	}
	return Match{}, fmt.Errorf("could not find match %s", fifaID)
}

type Match struct {
	Status   string    `json:"status"`
	ID       string    `json:"fifa_id"`
	HomeTeam string    `json:"home_team_country"`
	AwayTeam string    `json:"away_team_country"`
	Date     time.Time `json:"datetime"`
}

var cache map[string]string

func main() {
	cache = make(map[string]string)

	http.HandleFunc("/gameover", gameOver)
	http.HandleFunc("/newgame", deployNewGame)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func deployNewGame(w http.ResponseWriter, req *http.Request) {
	c, _ := ioutil.ReadAll(req.Body)

	m, err := findGameToSchedule(string(c))
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	f, err := ioutil.TempFile("", "")
	if err != nil {
		os.Exit(1)
	}

	createTeamSpec(m.HomeTeam, m, f)
	createTeamSpec(m.AwayTeam, m, f)
	cache[m.ID] = f.Name()

	f.Close()

	log.Println("***New Match: " + m.HomeTeam + " vs " + m.AwayTeam + "***")
	log.Println(f.Name())
	log.Println("-------------")
	cmd := exec.Command("kubectl", "-n", "wcawesome", "apply", "-f", f.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()
	cmd.Wait()
}

func gameOver(w http.ResponseWriter, req *http.Request) {
	c, _ := ioutil.ReadAll(req.Body)
	log.Println(string(c))
	log.Println("***Game Over: " + string(c) + "***")
	log.Println("-------------")
	f := cache[string(c)]
	cmd := exec.Command("kubectl", "-n", "wcawesome", "delete", "-f", f)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()
	cmd.Wait()
}

func findGameToSchedule(fifaID string) (Match, error) {
	var m MatchSet
	resp, err := http.Get("http://worldcup.sfg.io/matches")
	if err != nil {
		log.Fatal(err)
	}
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()

	err = json.Unmarshal(bodyBytes, &m)
	if err != nil {
		log.Fatal(err)
	}
	return m.Find(fifaID)
}

func createTeamSpec(teamName string, m Match, output io.Writer) string {
	const specTemplate = `---
apiVersion: v1
kind: Pod
metadata:
  name: {{.ID}}-{{machineReadable .Country}}-team
  labels:
    app: {{.ID}}-{{machineReadable .Country}}-team
    team: {{machineReadable .Country}}
spec:
  containers:
  - name: team
    image: smuthoo/wcawesome-game
    ports:
    - containerPort: 80
    env:
    - name: COUNTRY
      value: "{{.Country}}"
    - name: FIFA_ID
      value: "{{.ID}}"
    - name: REF_URL
      value: "{{.RefURL}}"
`
	t := template.Must(template.New("team").Funcs(template.FuncMap{
		"machineReadable": func(human string) string {
			return strings.ToLower(strings.Replace(human, " ", "-", -1))
		},
	}).Parse(specTemplate))
	t.Execute(output, struct {
		Country string
		ID      string
		RefURL  string
	}{
		Country: teamName,
		ID:      m.ID,
		RefURL:  os.Getenv("REF_URL"),
	})

	return ""
}
