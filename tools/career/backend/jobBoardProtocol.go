package main

import (
	"log"

	"career-tool-backend/applicationlog"
	"career-tool-backend/jobboard"
)

// attachProtocolSummaries fills the Protocol of every card that has entries in the profile's protocol (step 176). It is a side view,
// like the persona scores: when the entries cannot be read it logs that and leaves the cards as they are, so the board never fails
// because of it. A card without entries keeps no Protocol, so it looks as before.
func attachProtocolSummaries(board *jobboard.Board, profileID string) {
	summaries, err := applicationlog.Summaries(profileID)
	if err != nil {
		log.Printf("job board: could not read the protocol of profile %s: %v", profileID, err)
		return
	}
	for ci := range board.Columns {
		for ji := range board.Columns[ci].Jobs {
			card := &board.Columns[ci].Jobs[ji]
			if s, ok := summaries[card.JobID]; ok {
				card.Protocol = &s
			}
		}
	}
}
