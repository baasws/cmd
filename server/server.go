package main

import (
	"context"
	"log"
	"net/http"

	"github.com/briscola-as-a-service/game"
	"github.com/briscola-as-a-service/gosdk"
	"github.com/briscola-as-a-service/waitinglist"
	"github.com/frncscsrcc/longpoll"
	"github.com/frncscsrcc/resthelper"
)

// Global variables
var lp *longpoll.LongPoll
var wls *waitinglist.WaitingLists

var playerIDToDecker map[string]*game.Decker

func init() {
	playerIDToDecker = make(map[string]*game.Decker)
}

func getWaitingList(r *http.Request) string {
	// Search in URL
	waitingLists, ok := r.URL.Query()["type"]
	if ok == true && len(waitingLists) > 0 {
		return waitingLists[0]
	}
	return ""
}

func play(decker *game.Decker) {

}

func startGame(w http.ResponseWriter, r *http.Request) {
	// Check user session (it could be moved in middleware)
	sessionID := resthelper.GetSessionID(r)
	if sessionID == "" {
		resthelper.SendError(w, 400, "Missing SessionID")
		return
	}

	waitingListName := getWaitingList(r)
	if waitingListName == "" {
		resthelper.SendError(w, 400, "Missing game type")
		return
	}

	// Note feed = subscriptionID
	// PlayerID = subscriptionID
	subscriptionID := resthelper.GetNewToken(32)
	playerID := subscriptionID

	// Add the player to a waiting list
	err := wls.AddPlayer(waitingListName, playerID, playerID)
	if err != nil {
		resthelper.SendError(w, 400, err.Error())
		return
	}

	// Should start the game (is now waiting list full?)
	// deckerID is returned in any case and it is the game broadcast message
	deckerID, deckerPtr, err := wls.StartGame(waitingListName)

	// Inject a new subscriptionID in the request contex.
	// This will be passed to the longpool library, and the library will use this
	// value (instead of creating a random one) because there is only a feed,
	// subscriptionID = feed
	broadcastFeed := deckerID
	feeds := []string{subscriptionID, broadcastFeed}
	contextStruct := longpoll.ContextStruct{
		Feeds:          feeds,
		SubscriptionID: subscriptionID,
		SessionID:      sessionID,
	}
	ctx := context.WithValue(r.Context(), longpoll.ContextStructIdentifier, contextStruct)

	// Add the feed in longpoll object
	lp.AddFeeds(feeds)

	if err != nil {
		if err.Error() != "waiting for players" {
			resthelper.SendError(w, 500, err.Error())
			return
		}

		// Subscribe to the feed. Waiting to start the game.
		lp.SubscribeHandler(w, r.WithContext(ctx))

		lp.NewEvent(broadcastFeed, gosdk.PlayEvent{
			Message: playerID + " joined. Waiting for more players",
		})

		return
	}

	// Waiting list is complete. Game can start.
	players := deckerPtr.GetSortedPlayers()
	for _, player := range players {
		playerID := player.ID()
		playerIDToDecker[playerID] = deckerPtr
	}

	lp.SubscribeHandler(w, r.WithContext(ctx))

	// playerID = feed
	firstPlayerFeed := players[0].ID()

	lp.NewEvent(broadcastFeed, gosdk.PlayEvent{
		Message: playerID + " joined. Ready to play. " + firstPlayerFeed + " begins.",
	})

	// Send the cards to the first player
	lp.NewEvent(firstPlayerFeed, gosdk.PlayEvent{
		Cards:          *(deckerPtr.GetPlayerCards(players[0])),
		Briscola:       deckerPtr.GetBriscola(),
		ActionRequired: true,
	})

	return
}

func playGame(w http.ResponseWriter, r *http.Request) {
	// Check user session (it could be moved in middleware)
	sessionID := resthelper.GetSessionID(r)
	if sessionID == "" {
		resthelper.SendError(w, 400, "Missing sessionID")
		return
	}

	//This should be moved from here!
	subscriptionIDs, ok := r.URL.Query()["subscriptionID"]
	if ok == false || len(subscriptionIDs) == 0 {
		resthelper.SendError(w, 400, "Missing subscriptionID")
		return
	}

	// TODO

	lp.ListenHandler(w, r)
}

func main() {
	wls = waitinglist.New()
	var err error
	err = wls.AddList("TEST", 2)
	if err != nil {
		log.Fatal(err)
	}

	lp = longpoll.New()
	mux := http.NewServeMux()
	mux.HandleFunc("/start", startGame)
	mux.HandleFunc("/play", playGame)
	log.Println("Start server on port :8082")
	contextedMux := resthelper.AddSessionID(resthelper.LogRequest(mux))
	log.Fatal(http.ListenAndServe(":8082", contextedMux))
}

func show(i interface{}) {
	log.Printf("%+v\n", i)
}
