//go:generate go get github.com/gin-gonic/gin github.com/gin-contrib/cors github.com/googollee/go-socket.io
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

type Post struct {
	ID        interface{} `bson:"_id,omitempty" json:"_id,omitempty"`
	User      string      `bson:"user" json:"user"`
	Caption   string      `bson:"caption" json:"caption"`
	PhotoURL  string      `bson:"photoUrl" json:"photoUrl"`
	CreatedAt time.Time   `bson:"createdAt" json:"createdAt"`
}

type Message struct {
	Sender    string    `bson:"sender" json:"sender"`
	Receiver  string    `bson:"receiver" json:"receiver"`
	Message   string    `bson:"message" json:"message"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}

type Module struct {
	ID        interface{} `bson:"_id,omitempty" json:"_id,omitempty"`
	Title     string      `bson:"title" json:"title"`
	FileName  string      `bson:"fileName" json:"fileName"`
	FileURL   string      `bson:"fileUrl" json:"fileUrl"`
	FileType  string      `bson:"fileType" json:"fileType"`
	CreatedAt time.Time   `bson:"createdAt" json:"createdAt"`
}

var postsColl, messagesColl, modulesColl *mongo.Collection

func main() {
	_ = godotenv.Load()
	mongoURI := os.Getenv("MONGO_URI")
	dbName := os.Getenv("DB_NAME")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}

	// MongoDB connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}

	db := client.Database(dbName)
	postsColl = db.Collection("posts")
	messagesColl = db.Collection("messages")
	modulesColl = db.Collection("modules")
	log.Println("Connected to MongoDB:", dbName)

	// Socket.io for chat
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
		// broadcast
		server.BroadcastToRoom("/", "global", "receive_message", msg)
	})

	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		log.Println("Disconnected:", s.ID(), reason)
	})

	go server.Serve()
	defer server.Close()

	// Routes
	http.HandleFunc("/posts", cors(getPostsHandler))
	http.HandleFunc("/uploadPost", cors(uploadPostHandler))

	http.HandleFunc("/getMessages", cors(getMessagesHandler))
	http.HandleFunc("/sendMessage", cors(sendMessageHandler))

	http.HandleFunc("/modules", cors(getModulesHandler))
	http.HandleFunc("/uploadModule", cors(uploadModuleHandler))

	http.Handle("/socket.io/", server)

	log.Println("Server listening on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// -------------------- CORS --------------------
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

// -------------------- POSTS --------------------
func getPostsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cur, err := postsColl.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{"createdAt", -1}}))
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)
	var posts []Post
	if err := cur.All(ctx, &posts); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(posts)
}

func uploadPostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var p Post
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	p.CreatedAt = time.Now().UTC()
	_, err := postsColl.InsertOne(r.Context(), p)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// -------------------- CHAT --------------------
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
	json.NewEncoder(w).Encode(msgs)
}

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
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// -------------------- MODULES --------------------
func getModulesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cur, err := modulesColl.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{"createdAt", -1}}))
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)
	var modules []Module
	if err := cur.All(ctx, &modules); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(modules)
}

func uploadModuleHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "POST only", http.StatusMethodNotAllowed)
        return
    }

    var m Module
    if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
        log.Println("JSON decode error:", err)
        http.Error(w, "invalid body", http.StatusBadRequest)
        return
    }

    m.CreatedAt = time.Now()

    res, err := modulesColl.InsertOne(r.Context(), m)
    if err != nil {
        log.Println("Mongo insert error:", err)
        http.Error(w, "db error", http.StatusInternalServerError)
        return
    }

    log.Println("Module inserted ID:", res.InsertedID)
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}



