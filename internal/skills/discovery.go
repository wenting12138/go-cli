package skills

import (
	"sort"
	"strings"
	"unicode"
)

type discoveryCandidate struct {
	skill Skill
	score int
}

func Discover(skills []Skill, query string) (Skill, bool) {
	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return Skill{}, false
	}

	candidates := make([]discoveryCandidate, 0, len(skills))
	for _, skill := range skills {
		if !skill.UserInvocable {
			continue
		}
		score := discoveryScore(skill, queryTokens)
		if score <= 0 {
			continue
		}
		candidates = append(candidates, discoveryCandidate{
			skill: skill,
			score: score,
		})
	}
	if len(candidates) == 0 {
		return Skill{}, false
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].skill.Name < candidates[j].skill.Name
	})

	if candidates[0].score < 4 {
		return Skill{}, false
	}
	return candidates[0].skill, true
}

func discoveryScore(skill Skill, queryTokens []string) int {
	score := 0
	nameTokens := tokenize(skill.Name)
	descriptionTokens := tokenize(skill.Description)
	whenTokens := tokenize(skill.WhenToUse)
	pathTokens := tokenize(skill.Path)

	contentPreview := skill.Content
	if len(contentPreview) > 600 {
		contentPreview = contentPreview[:600]
	}
	contentTokens := tokenize(contentPreview)

	for _, token := range queryTokens {
		score += weightedTokenMatches(token, nameTokens, 6)
		score += weightedTokenMatches(token, descriptionTokens, 3)
		score += weightedTokenMatches(token, whenTokens, 4)
		score += weightedTokenMatches(token, pathTokens, 2)
		score += weightedTokenMatches(token, contentTokens, 1)

		if strings.EqualFold(skill.Name, token) {
			score += 8
		}
		if strings.Contains(strings.ToLower(skill.Name), token) {
			score += 2
		}
	}

	queryJoined := strings.Join(queryTokens, " ")
	nameJoined := strings.Join(nameTokens, " ")
	if queryJoined != "" && nameJoined != "" && (strings.Contains(nameJoined, queryJoined) || strings.Contains(queryJoined, nameJoined)) {
		score += 5
	}

	return score
}

func weightedTokenMatches(token string, corpus []string, weight int) int {
	if token == "" || len(corpus) == 0 || weight <= 0 {
		return 0
	}
	score := 0
	for _, word := range corpus {
		switch {
		case word == token:
			score += weight
		case len(token) >= 4 && strings.Contains(word, token):
			score += weight / 2
		}
	}
	return score
}

func tokenize(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return nil
	}

	parts := strings.FieldsFunc(text, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_')
	})

	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "-_")
		if len(part) < 2 {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	return out
}
