/**
 * @file game.go
 * @author Mikhail Klementyev jollheef<AT>riseup.net
 * @license GNU AGPLv3
 * @date November, 2015
 * @brief game api
 *
 * Contain functions for calculate score, check flag etc.
 */

package game

import (
	"database/sql"
	"log"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/jollheef/henhouse/db"
)

// Game struct
type Game struct {
	db              *sql.DB
	Start           time.Time
	End             time.Time
	OpenTimeout     time.Duration // after solve task
	AutoOpen        bool
	AutoOpenTimeout time.Duration // if task does not solved
	scoreboardLock  *sync.Mutex
	TaskPrice       struct {
		TeamsBase              float64
		P500, P400, P300, P200 float64
	}
}

// TaskInfo provide information about task
type TaskInfo struct {
	ID         int
	Name       string
	Desc       string
	Author     string
	Price      int
	Opened     bool
	Level      int
	SolvedBy   []int
	OpenedTime time.Time
}

// CategoryInfo provide information about categories and tasks
type CategoryInfo struct {
	Name      string
	TasksInfo []TaskInfo
}

// TeamScoreInfo provide information about team score
type TeamScoreInfo struct {
	ID    int
	Name  string
	Desc  string
	Score int
}

type byScore []TeamScoreInfo

func (tr byScore) Len() int           { return len(tr) }
func (tr byScore) Swap(i, j int)      { tr[i], tr[j] = tr[j], tr[i] }
func (tr byScore) Less(i, j int) bool { return tr[i].Score > tr[j].Score }

type byLevel []TaskInfo

func (ti byLevel) Len() int           { return len(ti) }
func (ti byLevel) Swap(i, j int)      { ti[i], ti[j] = ti[j], ti[i] }
func (ti byLevel) Less(i, j int) bool { return ti[i].Level < ti[j].Level }

// TaskPrice provide task price info

// NewGame create new game
func NewGame(database *sql.DB, start, end time.Time) (g Game, err error) {

	g.db = database
	g.Start = start
	g.End = end

	// Default values
	g.TaskPrice.P200 = 0.50
	g.TaskPrice.P300 = 0.30
	g.TaskPrice.P400 = 0.15
	g.TaskPrice.P500 = 0.10

	g.scoreboardLock = &sync.Mutex{}

	tasks, err := db.GetTasks(g.db)
	if err != nil {
		return
	}
	g.TaskPrice.TeamsBase = float64(len(tasks))

	err = g.RecalcScoreboard()
	if err != nil {
		return
	}

	return
}

// SetTaskPrice convert and set price of tasks
func (g *Game) SetTaskPrice(p500, p400, p300, p200 int) {
	g.TaskPrice.P200 = float64(p200) / 100
	g.TaskPrice.P300 = float64(p300) / 100
	g.TaskPrice.P400 = float64(p400) / 100
	g.TaskPrice.P500 = float64(p500) / 100
}

// SetTeamsBase force set amount of teams for calc price task
func (g *Game) SetTeamsBase(teams int) {
	g.TaskPrice.TeamsBase = float64(teams)
}

// Run open first level tasks and start auto open routine
func (g Game) Run() (err error) {

	for time.Now().Before(g.Start) {
		time.Sleep(time.Second)
	}

	cats, err := g.Tasks()
	if err != nil {
		return
	}

	for _, c := range cats {
		for _, t := range c.TasksInfo {
			log.Println("Open task", t.Name, t.Level)
			err = db.SetOpened(g.db, t.ID, true)
			if err != nil {
				return
			}

			break
		}
	}

	if !g.AutoOpen {
		return
	}

	go func() {
		for {
			time.Sleep(time.Second)
			err = g.autoOpenTasks()
			if err != nil {
				log.Println("Auto open tasks fail:", err)
			}
		}
	}()

	return
}

func (g Game) autoOpenTasks() (err error) {

	now := time.Now()

	cats, err := g.Tasks()
	if err != nil {
		return
	}

	for _, c := range cats {
		prev := TaskInfo{Opened: true}
		for i, t := range c.TasksInfo {
			if i == 0 || t.Opened || !prev.Opened {
				prev = t
				continue
			}

			if now.After(prev.OpenedTime.Add(g.AutoOpenTimeout)) {
				log.Println("Open task", t.Name, t.Level)
				err = db.SetOpened(g.db, t.ID, true)
				if err != nil {
					return
				}
			}

			prev = t
		}

	}

	return
}

func (g Game) taskPrice(database *sql.DB, taskID int) (price int, err error) {

	count, err := db.GetSolvedCount(database, taskID)

	fprice := float64(count) / g.TaskPrice.TeamsBase

	if fprice <= g.TaskPrice.P500 {
		price = 500
	} else if fprice <= g.TaskPrice.P400 {
		price = 400
	} else if fprice <= g.TaskPrice.P300 {
		price = 300
	} else if fprice <= g.TaskPrice.P200 {
		price = 200
	} else {
		price = 100
	}

	return
}

// Tasks returns categories with tasks
func (g Game) Tasks() (cats []CategoryInfo, err error) {

	tasks, err := db.GetTasks(g.db)
	if err != nil {
		return
	}

	categories, err := db.GetCategories(g.db)
	if err != nil {
		return
	}

	for _, category := range categories {

		cat := CategoryInfo{Name: category.Name}

		for _, task := range tasks {

			if task.CategoryID == category.ID {

				var price int
				price, err = g.taskPrice(g.db, task.ID)
				if err != nil {
					return
				}

				var solvedBy []int
				solvedBy, err = db.GetSolvedBy(g.db, task.ID)
				if err != nil {
					return
				}

				if !task.Opened {
					task.Desc = ""
				}

				tInfo := TaskInfo{
					ID:         task.ID,
					Name:       task.Name,
					Desc:       task.Desc,
					Price:      price,
					Opened:     task.Opened,
					SolvedBy:   solvedBy,
					Author:     task.Author,
					Level:      task.Level,
					OpenedTime: task.OpenedTime,
				}

				cat.TasksInfo = append(cat.TasksInfo, tInfo)
			}
		}

		sort.Sort(byLevel(cat.TasksInfo))

		cats = append(cats, cat)
	}

	return
}

// Scoreboard returns sorted scoreboard
func (g Game) Scoreboard() (scores []TeamScoreInfo, err error) {

	g.scoreboardLock.Lock()
	defer g.scoreboardLock.Unlock()

	teams, err := db.GetTeams(g.db)
	if err != nil {
		return
	}

	for _, team := range teams {

		if team.Test {
			continue
		}

		var s db.Score
		s, err = db.GetLastScore(g.db, team.ID)
		if err != nil {
			return
		}

		scores = append(scores,
			TeamScoreInfo{team.ID, team.Name, team.Desc, s.Score})
	}

	sort.Sort(byScore(scores))

	return
}

// RecalcScoreboard update scoreboard
func (g Game) RecalcScoreboard() (err error) {

	g.scoreboardLock.Lock()
	defer g.scoreboardLock.Unlock()

	teams, err := db.GetTeams(g.db)
	if err != nil {
		return
	}

	tasks, err := db.GetTasks(g.db)
	if err != nil {
		return
	}

	for _, team := range teams {

		if team.Test {
			continue
		}

		score := 0

		for _, task := range tasks {

			var price int
			price, err = g.taskPrice(g.db, task.ID)
			if err != nil {
				return
			}

			var solved bool
			solved, err = db.IsSolved(g.db, team.ID, task.ID)
			if err != nil {
				return
			}

			if solved {
				score += price
			}
		}

		err = db.AddScore(g.db, &db.Score{TeamID: team.ID, Score: score})
		if err != nil {
			return
		}
	}

	return
}

// OpenNextTask open next task by level
func (g Game) OpenNextTask(t db.Task) (err error) {

	time.Sleep(g.OpenTimeout)

	tasks, err := db.GetTasks(g.db)
	if err != nil {
		return
	}

	for _, task := range tasks {
		// If same category and next level
		if t.CategoryID == task.CategoryID && t.Level+1 == task.Level {
			// If not already opened
			if !task.Opened {
				// Open it!
				log.Println("Open task", t.Name, t.Level)
				err = db.SetOpened(g.db, task.ID, true)
				if err != nil {
					return
				}
			}
		}
	}

	return
}

func (g Game) isTestTeam(teamID int) bool {

	teams, err := db.GetTeams(g.db)
	if err != nil {
		log.Println("Get teams fail:", err)
		return true
	}

	for _, team := range teams {
		if team.ID == teamID {
			return team.Test
		}
	}

	return false
}

// Solve check flag for task and recalc scoreboard if flag correct
func (g Game) Solve(teamID, taskID int, flag string) (solved bool, err error) {

	tasks, err := db.GetTasks(g.db)
	if err != nil {
		return
	}

	for _, task := range tasks {
		if task.ID == taskID {

			solved, err = regexp.MatchString("^"+task.Flag+"$", flag)
			if err != nil {
				log.Println("Match regex fail:", err)
				return
			}

			if solved {

				if g.isTestTeam(teamID) {
					return
				}

				var isSolv bool // if already solved
				isSolv, err = db.IsSolved(g.db, teamID, taskID)
				if isSolv {
					return
				}

				now := time.Now()

				if now.After(g.Start) && now.Before(g.End) {
					err = db.AddFlag(g.db, &db.Flag{
						TeamID: teamID,
						TaskID: taskID,
						Flag:   flag,
						Solved: solved,
					})
					if err != nil {
						return
					}

					go g.OpenNextTask(task)
				}
			}

			break
		}
	}

	return
}
