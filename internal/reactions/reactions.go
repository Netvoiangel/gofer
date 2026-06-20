package reactions

import (
	"math/rand/v2"
	"strings"
	"unicode"
)

type Candidate struct {
	Text    string
	Topic   string
	Trigger string
	Chance  float64
}

func Match(text string, chattiness string, profanityLevel string) (Candidate, bool) {
	candidate, ok := CandidateFor(text, profanityLevel)
	if !ok {
		return Candidate{}, false
	}

	chance := chanceFor(chattiness)
	if rand.Float64() > chance {
		return Candidate{}, false
	}
	candidate.Chance = chance
	return candidate, true
}

func CandidateFor(text string, profanityLevel string) (Candidate, bool) {
	normalized := normalize(text)
	if normalized == "" || len([]rune(normalized)) > 40 {
		return Candidate{}, false
	}

	reply, topic, ok := replyFor(normalized, profanityLevel)
	if !ok {
		return Candidate{}, false
	}
	return Candidate{Text: reply, Topic: topic, Trigger: normalized}, true
}

func replyFor(text string, profanityLevel string) (string, string, bool) {
	switch text {
	case "супер", "супер!", "отлично", "кайф", "заебись":
		return "Супер — это когда прод не дымится. Всё остальное пока оптимизм.", "positive_short", true
	case "пиздец", "ппц", "жесть", "капец":
		return rude("Технически точное описание большинства инфраструктурных событий.", "Технически точное описание большинства инфраструктурных событий, мать его.", profanityLevel), "pain_short", true
	case "ого", "ничего себе", "ничоси":
		return "Вот это уже похоже на начало нормального инженерного безумия.", "surprise_short", true
	case "ghbdtn", "ghbdtn!", "прив", "привет":
		return "Привет, клавиатурный шаман. Раскладку опять унесло в прод?", "hello_layout", true
	case "понял", "поняла", "ясно", "ок", "окей":
		return "Фраза опасная. Обычно после неё начинается самое интересное.", "ack_short", true
	case "ахах", "ахахах", "хах", "хаха", "лол", "ору":
		return "Смех — это когда мозг ещё не признал, что это баг.", "laugh_short", true
	case "да", "ага", "угу":
		return "Угу. Записал в протокол сомнительной уверенности.", "yes_short", true
	case "нет", "неа":
		return "Отрицание принято. Осталось объяснить это реальности.", "no_short", true
	case "ладно":
		return "Вот с этого слова обычно начинается архитектурный компромисс с совестью.", "ok_short", true
	default:
		return "", "", false
	}
}

func chanceFor(chattiness string) float64 {
	switch strings.ToLower(strings.TrimSpace(chattiness)) {
	case "low":
		return 0.15
	case "medium":
		return 0.35
	case "insane":
		return 0.90
	default:
		return 0.60
	}
}

func rude(clean string, medium string, profanityLevel string) string {
	switch strings.ToLower(strings.TrimSpace(profanityLevel)) {
	case "none", "low":
		return clean
	default:
		return medium
	}
}

func normalize(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || r == '!' {
			return r
		}
		return -1
	}, text)
	return strings.Join(strings.Fields(text), " ")
}
