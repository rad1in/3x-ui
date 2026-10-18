package eventbus

// plainTitles names the events every channel can send as a title plus lines of
// text, so a new alert needs no per-channel formatting.
var plainTitles = map[EventType]string{
	EventResellerCreditLow: "Reseller credit is low",
	EventLoginNewLocation:  "New login location",
}

// PlainText renders an event as a title and text lines; ok is false for events
// with their own layout or whose data has no Lines method.
func PlainText(e Event) (title string, lines []string, ok bool) {
	title, known := plainTitles[e.Type]
	d, hasLines := e.Data.(interface{ Lines() []string })
	if !known || !hasLines {
		return "", nil, false
	}
	return title, d.Lines(), true
}
