//go:generate go get github.com/gin-gonic/gin github.com/gin-contrib/cors github.com/googollee/go-socket.io
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	socketio "github.com/googollee/go-socket.io"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// -------------------- STRUCTS --------------------

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

type Evaluation struct {
	ID          interface{} `bson:"_id,omitempty" json:"_id,omitempty"`
	StudentID   string      `bson:"studentId" json:"studentId"`
	Age         string      `bson:"age" json:"age"`
	GrossMotorB int         `bson:"grossMotorB" json:"grossMotorB"`
	GrossMotorE int         `bson:"grossMotorE" json:"grossMotorE"`
	FineMotorB  int         `bson:"fineMotorB" json:"fineMotorB"`
	FineMotorE  int         `bson:"fineMotorE" json:"fineMotorE"`
	SelfHelpB   int         `bson:"selfHelpB" json:"selfHelpB"`
	SelfHelpE   int         `bson:"selfHelpE" json:"selfHelpE"`
	ReceptiveB  int         `bson:"receptiveB" json:"receptiveB"`
	ReceptiveE  int         `bson:"receptiveE" json:"receptiveE"`
	ExpressiveB int         `bson:"expressiveB" json:"expressiveB"`
	ExpressiveE int         `bson:"expressiveE" json:"expressiveE"`
	CognitiveB  int         `bson:"cognitiveB" json:"cognitiveB"`
	CognitiveE  int         `bson:"cognitiveE" json:"cognitiveE"`
	SocialB     int         `bson:"socialB" json:"socialB"`
	SocialE     int         `bson:"socialE" json:"socialE"`
	CreatedAt   time.Time   `bson:"createdAt" json:"createdAt"`
}

// -------------------- GLOBALS --------------------

var db *mongo.Database
var postsColl, messagesColl, modulesColl, evalColl *mongo.Collection

// Track online users
var activeUsers = make(map[string]string) // socketID -> username
var mu sync.Mutex

// -------------------- MAIN --------------------

func main() {
	_ = godotenv.Load()
	mongoURI := os.Getenv("MONGO_URI")
	dbName := os.Getenv("DB_NAME")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	db = client.Database(dbName)
	postsColl = db.Collection("posts")
	messagesColl = db.Collection("messages")
	modulesColl = db.Collection("modules")
	evalColl = db.Collection("evaluations")

	log.Println("✅ Connected to MongoDB:", dbName)

	// -------------------- SOCKET.IO --------------------
	server := socketio.NewServer(nil)

	server.OnConnect("/", func(s socketio.Conn) error {
		log.Println("🟢 New connection:", s.ID())
		return nil
	})

	server.OnEvent("/", "user_online", func(s socketio.Conn, username string) {
		mu.Lock()
		activeUsers[s.ID()] = username
		mu.Unlock()
		server.BroadcastToRoom("/", "", "active_users", getActiveUserList())
		log.Println("👤", username, "is online")
	})

	server.OnEvent("/", "send_message", func(s socketio.Conn, msg Message) {
		msg.CreatedAt = time.Now()
		_, err := messagesColl.InsertOne(context.Background(), msg)
		if err != nil {
			log.Println("❌ DB insert error:", err)
			return
		}
		server.BroadcastToRoom("/", "", "receive_message", msg)
	})

	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		mu.Lock()
		username := activeUsers[s.ID()]
		delete(activeUsers, s.ID())
		mu.Unlock()
		server.BroadcastToRoom("/", "", "active_users", getActiveUserList())
		log.Println("🔴", username, "disconnected:", reason)
	})

	go server.Serve()
	defer server.Close()

	// -------------------- ROUTES --------------------
	http.HandleFunc("/posts", cors(getPostsHandler))
	http.HandleFunc("/uploadPost", cors(uploadPostHandler))

	http.HandleFunc("/getMessages", cors(getMessagesHandler))
	http.HandleFunc("/sendMessage", cors(sendMessageHandler))

	http.HandleFunc("/modules", cors(getModulesHandler))
	http.HandleFunc("/uploadModule", cors(uploadModuleHandler))
	http.HandleFunc("/file/", cors(serveFileHandler))

	http.HandleFunc("/addEvaluation", cors(addEvaluationHandler))
	http.HandleFunc("/evaluations/", cors(getEvaluationsHandler))

	http.Handle("/socket.io/", server)

	log.Println("🚀 Server running on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// -------------------- UTILS --------------------

func getActiveUserList() []string {
	mu.Lock()
	defer mu.Unlock()
	users := []string{}
	for _, u := range activeUsers {
		users = append(users, u)
	}
	return users
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

// -------------------- MODULES (GridFS) --------------------

func uploadModuleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file missing", http.StatusBadRequest)
		return
	}
	defer file.Close()

	bucket, err := gridfs.NewBucket(db)
	if err != nil {
		http.Error(w, "GridFS error", http.StatusInternalServerError)
		return
	}

	uploadStream, err := bucket.OpenUploadStream(header.Filename)
	if err != nil {
		http.Error(w, "Upload stream error", http.StatusInternalServerError)
		return
	}
	defer uploadStream.Close()

	_, err = io.Copy(uploadStream, file)
	if err != nil {
		http.Error(w, "File upload failed", http.StatusInternalServerError)
		return
	}

	fileID := uploadStream.FileID.(primitive.ObjectID)
	fileURL := fmt.Sprintf("https://publicbackend-production.up.railway.app/file/%s", fileID.Hex())

	module := Module{
		Title:     title,
		FileName:  header.Filename,
		FileURL:   fileURL,
		FileType:  header.Header.Get("Content-Type"),
		CreatedAt: time.Now(),
	}

	_, err = modulesColl.InsertOne(r.Context(), module)
	if err != nil {
		http.Error(w, "DB insert error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "fileUrl": fileURL})
}

func getModulesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cur, err := modulesColl.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{"createdAt", -1}}))
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)

	var modules []Module
	if err := cur.All(ctx, &modules); err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(modules)
}

func serveFileHandler(w http.ResponseWriter, r *http.Request) {
	idHex := r.URL.Path[len("/file/"):]
	objID, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.Error(w, "invalid file ID", http.StatusBadRequest)
		return
	}

	bucket, _ := gridfs.NewBucket(db)
	stream, err := bucket.OpenDownloadStream(objID)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer stream.Close()

	fileInfo := stream.GetFile()
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", fileInfo.Name))
	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, stream)
}

// -------------------- EVALUATIONS --------------------

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
	if studentID == "" {
		http.Error(w, "studentId missing", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cur, err := evalColl.Find(ctx, bson.M{"studentId": studentID})
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)

	var evals []Evaluation
	if err := cur.All(ctx, &evals); err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(evals)
}
