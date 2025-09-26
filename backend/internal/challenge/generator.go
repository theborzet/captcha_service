package challenge

import (
	"fmt"
	"html/template"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/theborzet/captcha_service/pkg/utils"
)

const (
	dragDropTemplate = `
		<div id="captcha-container" data-challenge-id="{{.ChallengeID}}" data-target-x="{{.TargetX}}" data-target-y="{{.TargetY}}" data-complexity="{{.Complexity}}">
		<div id="target" style="position: absolute; left: {{.TargetX}}px; top: {{.TargetY}}px; width: {{printf "%.0f" (div 60 (int .Complexity))}}px; height: {{printf "%.0f" (div 60 (int .Complexity))}}px;"></div>
		<div id="draggable" style="position: absolute; left: 0px; top: 0px; width: 60px; height: 60px;"></div>
		</div>
	`
	answer = "success"
)

func GenerateDragDropChallenge(store *ChallengeStore, complexity int) (string, string, error) {
	challengeID := utils.GenerateChallengeID()
	rand.Seed(time.Now().UnixNano())
	targetX := rand.Intn(260)
	targetY := rand.Intn(140)

	store.Set(challengeID, answer, targetX, targetY, complexity, 5*time.Minute)

	// Создаём новый шаблон и регистрируем функции
	tmpl := template.New("captcha").Funcs(template.FuncMap{
		"div": func(a, b int) float64 {
			if b == 0 {
				return 0
			}
			return float64(a) / float64(b)
		},
		"printf": func(format string, a ...interface{}) string {
			return fmt.Sprintf(format, a...)
		},
		"int": func(v interface{}) int {
			switch v := v.(type) {
			case int:
				return v
			case string:
				i, _ := strconv.Atoi(v)
				return i
			default:
				return 0
			}
		},
	})
	tmpl, err := tmpl.Parse(dragDropTemplate)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse template: %v", err)
	}

	var htmlBuilder strings.Builder
	err = tmpl.Execute(&htmlBuilder, map[string]interface{}{
		"ChallengeID": challengeID,
		"TargetX":     strconv.Itoa(targetX),
		"TargetY":     strconv.Itoa(targetY),
		"Complexity":  strconv.Itoa(complexity),
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to execute template: %v", err)
	}

	html := htmlBuilder.String()
	return challengeID, html, nil
}
