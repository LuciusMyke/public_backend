package main

import (
	"context"
	"encoding/json"
	
	"log"
	"net/http"
	"os"
	"time"

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

var postsColl *mongo.Collection

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, continuing...")
	}

	// Get values from .env
	mongoURI := os.Getenv("MONGO_URI")
	dbName := os.Getenv("DB_NAME")
	port := os.Getenv("PORT")

	if mongoURI == "" {
		log.Fatal("Missing MONGO_URI in .env")
	}
	if dbName == "" {
		dbName = "mydb"
	}
	if port == "" {
		port = "8080"
	}

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}

	postsColl = client.Database(dbName).Collection("posts")
	log.Println("Connected to MongoDB:", dbName)

	// Routes
	http.HandleFunc("/uploadPost", cors(uploadPostHandler))
	http.HandleFunc("/posts", cors(getPostsHandler))

	// Start server
	log.Println("Listening on :" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For demo allow all origins. In production restrict origins.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
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

func getPostsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cur, err := postsColl.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{"createdAt", -1}}).SetLimit(100))
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
