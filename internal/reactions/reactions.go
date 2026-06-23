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
	Backoff bool
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
		if candidate, ok := containedCandidate(normalized, profanityLevel); ok {
			return candidate, true
		}
		return Candidate{}, false
	}

	reply, topic, ok := replyFor(normalized, profanityLevel)
	if !ok {
		if candidate, ok := containedCandidate(normalized, profanityLevel); ok {
			return candidate, true
		}
		return Candidate{}, false
	}
	return Candidate{Text: reply, Topic: topic, Trigger: normalized, Backoff: isBackoffTopic(topic)}, true
}

func containedCandidate(text string, profanityLevel string) (Candidate, bool) {
	for _, phrase := range []string{
		"иди нахуй",
		"пойти ли тебе нахуй",
		"пошел нахуй",
		"пошёл нахуй",
		"не отвлекай",
		"не выебывайся",
		"заткнись",
		"хуесос",
		"дегенерат",
		"дебил",
	} {
		if strings.Contains(text, phrase) {
			reply, topic, ok := replyFor(phrase, profanityLevel)
			if ok {
				return Candidate{Text: reply, Topic: topic, Trigger: phrase, Backoff: isBackoffTopic(topic)}, true
			}
		}
	}
	return Candidate{}, false
}

func replyFor(text string, profanityLevel string) (string, string, bool) {
	switch text {
	case "супер", "супер!", "отлично", "кайф", "заебись", "круто":
		return "Круто — это когда эта радость не развалится через три сообщения, но я в вас почти не верю.", "positive_short", true
	case "пиздец", "ппц", "жесть", "капец":
		return rude("Технически точное описание большинства инфраструктурных событий.", "Технически точное описание большинства инфраструктурных событий, мать его. Кратко, честно, как аварийный лог.", profanityLevel), "pain_short", true
	case "ого", "ничего себе", "ничоси":
		return "Вот это уже похоже на начало нормального инженерного безумия.", "surprise_short", true
	case "ghbdtn", "ghbdtn!", "прив", "привет":
		return "Привет, клавиатурный шаман. Раскладку опять унесло в прод?", "hello_layout", true
	case "понял", "поняла", "ясно", "ок", "окей":
		return "Фраза опасная. Обычно после неё начинается самое интересное.", "ack_short", true
	case "ахах", "ахахах", "ахахаха", "хах", "хаха", "хахаха", "лол", "ору":
		return "Смех — это когда мозг ещё не признал, что это баг.", "laugh_short", true
	case "да", "ага", "угу":
		return "Угу. Записал в протокол сомнительной уверенности.", "yes_short", true
	case "нет", "неа":
		return "Отрицание принято. Осталось объяснить это реальности.", "no_short", true
	case "ладно", "нуладно", "ну ладно":
		return "Вот с этого слова обычно начинается архитектурный компромисс с совестью.", "ok_short", true
	case "иди нахуй", "да иди нахуй", "пойти ли тебе нахуй", "пошел нахуй", "пошёл нахуй", "иди отсюда", "выйди из чата":
		return rude("Принял, отхожу от микрофона.", "Принял, отхожу от микрофона. Без ответного цирка.", profanityLevel), "backoff_short", true
	case "не отвлекай", "не отвлекай меня", "не пиши", "заткнись", "молчи", "не выебывайся":
		return rude("Принял. Ухожу в тень.", "Принял. Ухожу в тень, без лишнего грохота.", profanityLevel), "backoff_short", true
	case "дебил", "дегенерат", "урод", "хуесос":
		return rude("Принял критику формы. Содержательно помолчу.", "Принял критику формы. Содержательно помолчу, а то сейчас опять разнесёт.", profanityLevel), "backoff_short", true
	default:
		return "", "", false
	}
}

func isBackoffTopic(topic string) bool {
	return topic == "backoff_short"
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
