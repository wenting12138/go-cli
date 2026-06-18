package domain

type Message struct {
	Role    string
	Content string
}

type Session struct {
	ID       string
	Title    string
	Messages []Message
}
