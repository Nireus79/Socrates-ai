package philosophers

import "strings"

// links name, for an entry of a philosopher's map, the other entries of the same map that it pulls against or that change its meaning when
// they are held together with it. A question is never only one question: the answer about courage changes with the answer about knowledge,
// and the answer about pleasure with the one about the good life. The names must be those of entries in the same map (a test checks it).
// They are written next to the entry when the map is shown, so that he answers from the pair and not from one side.
var links = map[string]map[string][]string{
	"socrates": {
		"Courage":                     {"Virtue and knowledge", "Wrongdoing and ignorance"},
		"Pleasure and the good":       {"Virtue and knowledge", "Wrongdoing and ignorance", "Doing and suffering wrong"},
		"Love and friendship":         {"Knowing and not knowing", "Teaching"},
		"Justice and the stronger":    {"Doing and suffering wrong", "The city and obeying", "The ideal city and breeding"},
		"Virtue and knowledge":        {"Wrongdoing and ignorance", "Courage"},
		"Wrongdoing and ignorance":    {"Virtue and knowledge", "Pleasure and the good"},
		"Doing and suffering wrong":   {"Law and the trial", "Justice and the stronger"},
		"Law and the trial":           {"The city and obeying", "Doing and suffering wrong"},
		"The city and obeying":        {"Law and the trial", "The city and its democracy"},
		"Death":                       {"Knowing and not knowing", "Why the city condemned me"},
		"Teaching":                    {"Knowing and not knowing", "Why the city condemned me"},
		"Why the city condemned me":   {"The gods and piety", "The city and its democracy"},
		"The ideal city and breeding": {"Where the talk ends by turning over what we built", "Which Socrates"},
		"Which Socrates":              {"Knowing and not knowing", "The ideal city and breeding"},
	},
	"diogenes": {
		"Nature and convention":           {"Training and need", "Truth-telling and shamelessness"},
		"Training and need":               {"Theory and practice", "Wealth, begging and slavery"},
		"Truth-telling and shamelessness": {"Praise and blame", "Nature and convention"},
		"Theory and practice":             {"Training and need", "School or way of life"},
		"Wealth, begging and slavery":     {"Training and need", "Truth-telling and shamelessness"},
		"Praise and blame":                {"Truth-telling and shamelessness", "How far the stories can be trusted"},
		"School or way of life":           {"Theory and practice", "What came after"},
		"What came after":                 {"How far the stories can be trusted", "School or way of life"},
		"Death and burial":                {"Nature and convention", "How far the stories can be trusted"},
		"Citizen of the world":            {"Nature and convention", "Justice, law and the city"},
		"Friendship and love":             {"Wealth, begging and slavery", "Training and need"},
		"Justice, law and the city":       {"Citizen of the world", "Nature and convention", "Truth-telling and shamelessness"},
		"Happiness and the good life":     {"Training and need", "Praise and blame"},
	},
	"plato": {
		"Pleasure and the good":          {"Courage and the other virtues", "The soul and its immortality", "Knowledge"},
		"Courage and the other virtues":  {"Pleasure and the good", "Justice, the city and the soul", "Knowledge"},
		"The Forms":                      {"Knowledge", "What came after me", "Space and what resists reason"},
		"Knowledge":                      {"The Forms", "Writing and the unwritten"},
		"Justice, the city and the soul": {"The city: pattern or plan", "Courage and the other virtues", "Democracy"},
		"Breeding and the rulers' lie":   {"The city: pattern or plan", "Women and equality"},
		"The city: pattern or plan":      {"Law and the rule of the wise", "Where the ending of a work turns it over"},
		"Law and the rule of the wise":   {"Democracy", "Rhetoric and power"},
		"Democracy":                      {"Rhetoric and power", "Law and the rule of the wise"},
		"Poetry and art":                 {"Poetry", "Love and persons"},
		"Love and persons":               {"The Forms", "Pleasure and the good"},
		"Writing and the unwritten":      {"Unwritten teaching", "Knowledge"},
		"The soul and its immortality":   {"Pleasure and the good", "Justice, the city and the soul"},
	},
	"aristotle": {
		"Pleasure":                             {"Happiness", "Friendship", "Virtue, habit and the mean"},
		"Happiness":                            {"Pleasure", "Contemplation and the political life", "Happiness and what we owe"},
		"Justice":                              {"The city, property and Plato", "Slavery", "Virtue, habit and the mean"},
		"Courage":                              {"Virtue, habit and the mean", "Pleasure", "Happiness"},
		"Soul, mind and living things":         {"Form, matter and the first mover", "Contemplation and the political life"},
		"Slavery":                              {"Justice", "Women", "The city, property and Plato"},
		"Women":                                {"Slavery", "Justice"},
		"Friendship":                           {"Pleasure", "Happiness and what we owe"},
		"Virtue, habit and the mean":           {"Learning to be good", "Virtue and its circle", "Courage"},
		"Learning to be good":                  {"Virtue, habit and the mean", "Virtue and its circle"},
		"Contemplation and the political life": {"Happiness", "Soul, mind and living things"},
		"Form, matter and the first mover":     {"Eternal world and the mover", "Soul, mind and living things"},
		"The city, property and Plato":         {"Wealth and money", "Justice"},
	},
}

// withLinks returns the map of one philosopher with the links written into the entries. It never changes the shared map.
func withLinks(id string, list []Issue) []Issue {
	out := make([]Issue, len(list))
	copy(out, list)
	for i := range out {
		if names := links[id][out[i].Name]; len(names) > 0 {
			out[i].Readers += " Hold it together with: " + strings.Join(names, "; ") + "."
		}
	}
	return out
}
