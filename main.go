package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	socketio "github.com/googollee/go-socket.io"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Message struct {
	Sender    string    `bson:"sender" json:"sender"`
	Receiver  string    `bson:"receiver" json:"receiver"`
	Message   string    `bson:"message" json:"message"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}

var messagesColl *mongo.Collection

func main() {
	_ = godotenv.Load()
	mongoURI := os.Getenv("MONGO_URI")
	dbName := os.Getenv("DB_NAME")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}

	// Connect MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	messagesColl = client.Database(dbName).Collection("messages")
	log.Println("Connected to MongoDB:", dbName)

	// Setup Socket.io server
	server := socketio.NewServer(nil)
	server.OnConnect("/", func(s socketio.Conn) error {
		log.Println("New connection:", s.ID())
		s.Join("global")
		return nil
	})

	server.OnEvent("/", "send_message", func(s socketio.Conn, msg Message) {
		msg.CreatedAt = time.Now()
		_, err := messagesColl.InsertOne(context.Background(), msg)
		if err != nil {
			log.Println("DB insert error:", err)
			return
		}
		// Broadcast message to all clients
		server.BroadcastToRoom("/", "global", "receive_message", msg)
	})

	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		log.Println("Disconnected:", s.ID(), reason)
	})

	go server.Serve()
	defer server.Close()

	// Timeline routes remain the same
	http.HandleFunc("/posts", cors(getPostsHandler))
	http.HandleFunc("/uploadPost", cors(uploadPostHandler))

	// Chatbot REST endpoints
	http.HandleFunc("/getMessages", cors(getMessagesHandler))
	http.HandleFunc("/sendMessage", cors(sendMessageHandler))
	http.Handle("/socket.io/", server)

	log.Println("Server listening on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

// Get all messages
func getMessagesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cur, err := messagesColl.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{"createdAt", 1}}))
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)

	var msgs []Message
	if err := cur.All(ctx, &msgs); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}

// Send message via REST
func sendMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var msg Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	msg.CreatedAt = time.Now()
	_, err := messagesColl.InsertOne(r.Context(), msg)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
