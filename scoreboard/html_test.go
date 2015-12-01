/**
 * @file html_test.go
 * @author Mikhail Klementyev jollheef<AT>riseup.net
 * @license GNU GPLv3
 * @date December, 2015
 * @brief test html helpers
 */

package scoreboard

import (
	"github.com/jollheef/henhouse/game"
	"testing"
)

func TestTaskToHTML(*testing.T) {
	html := taskToHTML(game.TaskInfo{})
	testMatch("Task is closed", html)

	html = taskToHTML(game.TaskInfo{Opened: true})
	testNotMatch("Task is closed", html)
}

func TestCategoryToHTML(*testing.T) {

	cat := game.CategoryInfo{}

	cat.TasksInfo = append(cat.TasksInfo, game.TaskInfo{})

	html := categoryToHTML(cat)
	if html != `<div class="col-xs-3"> <h1></h1><p><a`+
		` class="btn btn-default" disabled="disabled"`+
		` title="Task is closed">0. </a></p></div>` {
		panic(html)
	}
}
