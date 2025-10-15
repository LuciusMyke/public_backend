//go:generate go get github.com/gin-gonic/gin github.com/gin-contrib/cors
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

// ===== STRUCTS =====
type Post struct {
	ID        interface{} `bson:"_id,omitempty" json:"_id,omitempty"`
	Title     string      `bson:"title" json:"title"`
	Content   string      `bson:"content" json:"content"`
	ImageURL  string      `bson:"imageUrl" json:"imageUrl"`
	CreatedAt time.Time   `bson:"createdAt" json:"createdAt"`
}

type Message struct {
	ID        interface{} `bson:"_id,omitempty" json:"_id,omitempty"`
	Sender    string      `bson:"sender" json:"sender"`
	Content   string      `bson:"content" json:"content"`
	Timestamp time.Time   `bson:"timestamp" json:"timestamp"`
}

type Module struct {
	ID          interface{} `bson:"_id,omitempty" json:"_id,omitempty"`
	Title       string      `bson:"title" json:"title"`
	Description string      `bson:"description" json:"description"`
	FileURL     string      `bson:"fileUrl" json:"fileUrl"`
	CreatedAt   time.Time   `bson:"createdAt" json:"createdAt"`
}

type Evaluation struct {
	ID        interface{} `bson:"_id,omitempty" json:"_id,omitempty"`
	StudentID string      `bson:"studentId" json:"studentId"`
	Age       string      `bson:"age" json:"age"`
	GrossB    int         `bson:"grossB" json:"grossB"`
	GrossE    int         `bson:"grossE" json:"grossE"`
	FineB     int         `bson:"fineB" json:"fineB"`
	FineE     int         `bson:"fineE" json:"fineE"`
	SocialB   int         `bson:"socialB" json:"socialB"`
	SocialE   int         `bson:"socialE" json:"socialE"`
	CreatedAt time.Time   `bson:"createdAt" json:"createdAt"`
}

// ===== GLOBALS =====
var client *mongo.Client
var postColl, msgColl, moduleColl, evalColl *mongo.Collection

// ===== UTIL =====
func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			return
		}
		next.ServeHTTP(w, r)
	}
}

// ====== POSTS ======
func getPostsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cur, err := postColl.Find(ctx, bson.M{})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer cur.Close(ctx)

	var posts []Post
	cur.All(ctx, &posts)
	json.NewEncoder(w).Encode(posts)
}

func uploadPostHandler(w http.ResponseWriter, r *http.Request) {
	var post Post
	if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	post.CreatedAt = time.Now()

	_, err := postColl.InsertOne(r.Context(), post)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ====== MESSAGES ======
func getMessagesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cur, err := msgColl.Find(ctx, bson.M{})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer cur.Close(ctx)

	var messages []Message
	cur.All(ctx, &messages)
	json.NewEncoder(w).Encode(messages)
}

func sendMessageHandler(w http.ResponseWriter, r *http.Request) {
	var msg Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	msg.Timestamp = time.Now()

	_, err := msgColl.InsertOne(r.Context(), msg)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}

// ===== MODULES =====
func getModulesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cur, err := moduleColl.Find(ctx, bson.M{})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer cur.Close(ctx)

	var modules []Module
	cur.All(ctx, &modules)
	json.NewEncoder(w).Encode(modules)
}

func uploadModuleHandler(w http.ResponseWriter, r *http.Request) {
	var module Module
	if err := json.NewDecoder(r.Body).Decode(&module); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	module.CreatedAt = time.Now()

	_, err := moduleColl.InsertOne(r.Context(), module)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ===== EVALUATIONS =====
func addEvaluationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var eval Evaluation
	if err := json.NewDecoder(r.Body).Decode(&eval); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	eval.CreatedAt = time.Now()

	_, err := evalColl.InsertOne(r.Context(), eval)
	if err != nil {
		http.Error(w, "DB insert error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func getEvaluationsHandler(w http.ResponseWriter, r *http.Request) {
	studentID := r.URL.Path[len("/evaluations/"):]
	filter := bson.M{}
	if studentID != "" {
		filter["studentId"] = studentID
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cur, err := evalColl.Find(ctx, filter)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)

	var evaluations []Evaluation
	if err := cur.All(ctx, &evaluations); err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(evaluations)
}

// ===== MAIN =====
func main() {
	_ = godotenv.Load()
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb+srv://admin_mike:mike_admin10203@cluster0.8yqjlfb.mongodb.net/?retryWrites=true&w=majority&appName=Cluster0"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, _ = mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))

	db := client.Database("admin1")
	postColl = db.Collection("posts")
	msgColl = db.Collection("messages")
	moduleColl = db.Collection("modules")
	evalColl = db.Collection("evaluations")

	http.HandleFunc("/posts", cors(getPostsHandler))
	http.HandleFunc("/uploadPost", cors(uploadPostHandler))
	http.HandleFunc("/messages", cors(getMessagesHandler))
	http.HandleFunc("/sendMessage", cors(sendMessageHandler))
	http.HandleFunc("/modules", cors(getModulesHandler))
	http.HandleFunc("/uploadModule", cors(uploadModuleHandler))
	http.HandleFunc("/addEvaluation", cors(addEvaluationHandler))
	http.HandleFunc("/evaluations/", cors(getEvaluationsHandler))

	log.Println("✅ Server running on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
