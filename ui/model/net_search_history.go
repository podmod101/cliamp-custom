package model

// browseYouTubeSearchHistory keeps the unfinished query while walking through
// history. Recalled queries remain editable, with the cursor at the end.
func (m *Model) browseYouTubeSearchHistory(older bool) {
	s := &m.netSearch
	if len(s.history) == 0 {
		return
	}
	if older {
		if s.historyPos == len(s.history) {
			return
		}
		if s.historyPos == 0 {
			s.draft = s.query
		}
		s.historyPos++
	} else {
		if s.historyPos == 0 {
			return
		}
		s.historyPos--
	}
	if s.historyPos == 0 {
		s.query = s.draft
	} else {
		s.query = s.history[s.historyPos-1]
	}
	s.err = ""
	m.textInput = textEditor{field: "net-search", cursor: len(s.query)}
}
