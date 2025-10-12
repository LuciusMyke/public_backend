//go:generate go get github.com/googollee/go-socket.io github.com/joho/godotenv
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
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

type Student struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"_id,omitempty"`
	Name      string             `bson:"name" json:"name"`
	Email     string             `bson:"email" json:"email"`
	Password  string             `bson:"password" json:"password"` // plain here for simplicity (not recommended in prod)
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

type Session struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"_id,omitempty"`
	Token     string             `bson:"token" json:"token"`
	StudentID primitive.ObjectID `bson:"studentId" json:"studentId"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

type Post struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"_id,omitempty"`
	User      string             `bson:"user" json:"user"`
	Caption   string             `bson:"caption" json:"caption"`
	PhotoURL  string             `bson:"photoUrl" json:"photoUrl"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

type Message struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"_id,omitempty"`
	Sender      string             `bson:"sender" json:"sender"`       // "student" or "admin"
	Receiver    string             `bson:"receiver" json:"receiver"`   // "admin" or student id
	StudentID   primitive.ObjectID `bson:"studentId" json:"studentId"` // ties message to student account
	StudentName string             `bson:"studentName" json:"studentName"`
	Message     string             `bson:"message" json:"message"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
}

type Module struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"_id,omitempty"`
	Title     string             `bson:"title" json:"title"`
	FileName  string             `bson:"fileName" json:"fileName"`
	FileURL   string             `bson:"fileUrl" json:"fileUrl"`
	FileType  string             `bson:"fileType" json:"fileType"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

type Evaluation struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"_id,omitempty"`
	StudentID string             `bson:"studentId" json:"studentId"`
	Age       string             `bson:"age" json:"age"`

	GrossMotorB int `bson:"grossMotorB" json:"grossMotorB"`
	GrossMotorE int `bson:"grossMotorE" json:"grossMotorE"`

	FineMotorB int `bson:"fineMotorB" json:"fineMotorB"`
	FineMotorE int `bson:"fineMotorE" json:"fineMotorE"`

	SelfHelpB int `bson:"selfHelpB" json:"selfHelpB"`
	SelfHelpE int `bson:"selfHelpE" json:"selfHelpE"`

	ReceptiveB int `bson:"receptiveB" json:"receptiveB"`
	ReceptiveE int `bson:"receptiveE" json:"receptiveE"`

	ExpressiveB int `bson:"expressiveB" json:"expressiveB"`
	ExpressiveE int `bson:"expressiveE" json:"expressiveE"`

	CognitiveB int `bson:"cognitiveB" json:"cognitiveB"`
	CognitiveE int `bson:"cognitiveE" json:"cognitiveE"`

	SocialB int `bson:"socialB" json:"socialB"`
	SocialE int `bson:"socialE" json:"socialE"`

	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}

// -------------------- GLOBALS --------------------

var db *mongo.Database
var studentsColl, sessionsColl, postsColl, messagesColl, modulesColl, evalColl *mongo.Collection

// socket mapping: socketID -> studentID (hex)
var socketStudent = map[string]string{}

func main() {
	_ = godotenv.Load()
	mongoURI := os.Getenv("MONGO_URI")
	dbName := os.Getenv("DB_NAME")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}
	if mongoURI == "" {
		log.Fatal("Missing MONGO_URI in environment")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}

	db = client.Database(dbName)
	studentsColl = db.Collection("students")
	sessionsColl = db.Collection("sessions")
	postsColl = db.Collection("posts")
	messagesColl = db.Collection("messages")
	modulesColl = db.Collection("modules")
	evalColl = db.Collection("evaluations")

	log.Println("✅ Connected to MongoDB:", dbName)

	// SOCKET.IO
	server := socketio.NewServer(nil)

	server.OnConnect("/", func(s socketio.Conn) error {
		log.Println("Socket connect:", s.ID())
		return nil
	})

	// student tells server they're active (includes token)
	// payload: { token: "<session-token>" }
	server.OnEvent("/", "student_active", func(s socketio.Conn, data map[string]string) {
		token := data["token"]
		if token == "" {
			s.Emit("error", "missing token")
			return
		}
		// find session
		var sess Session
		err := sessionsColl.FindOne(context.Background(), bson.M{"token": token}).Decode(&sess)
		if err != nil {
			s.Emit("error", "invalid session")
			return
		}
		studentIDHex := sess.StudentID.Hex()
		socketStudent[s.ID()] = studentIDHex
		// join room for this student and global admin room
		s.Join("student_"+studentIDHex)
		s.Join("global")
		log.Printf("Socket %s mapped to student %s\n", s.ID(), studentIDHex)

		// send chat history for this student
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		cur, err := messagesColl.Find(ctx2, bson.M{"studentId": sess.StudentID}, options.Find().SetSort(bson.D{{"createdAt", 1}}))
		if err == nil {
			var msgs []Message
			_ = cur.All(ctx2, &msgs)
			s.Emit("chat_history", msgs)
		}
	})

	// admin can join and listen, or send messages
	// send_message is used widely: for socket we accept a Message struct (sender/receiver/studentId/studentName/message)
	server.OnEvent("/", "send_message", func(s socketio.Conn, incoming Message) {
		// ensure StudentID is provided for student messages
		if incoming.StudentID.IsZero() {
			// if receiver contains student id hex, attempt parse
			// but require studentId for persistence
			log.Println("incoming message missing studentId")
			return
		}
		incoming.CreatedAt = time.Now()
		_, err := messagesColl.InsertOne(context.Background(), incoming)
		if err != nil {
			log.Println("DB insert error:", err)
			return
		}
		// broadcast to global (so admin & all devices see it) and specifically to student room
		server.BroadcastToRoom("/", "global", "receive_message", incoming)
		server.BroadcastToRoom("/", "student_"+incoming.StudentID.Hex(), "receive_message", incoming)
	})

	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		log.Println("Socket disconnect:", s.ID(), "reason:", reason)
		delete(socketStudent, s.ID())
	})

	go server.Serve()
	defer server.Close()

	// REST routes
	http.HandleFunc("/register", cors(registerHandler))
	http.HandleFunc("/login", cors(loginHandler))
	http.HandleFunc("/students", cors(listStudentsHandler)) // admin: list students + basic info

	http.HandleFunc("/posts", cors(getPostsHandler))
	http.HandleFunc("/uploadPost", cors(uploadPostHandler))
	http.HandleFunc("/deletePost", cors(deletePostHandler))

	http.HandleFunc("/getMessages", cors(getMessagesHandler))
	http.HandleFunc("/sendMessage", cors(sendMessageHandler)) // REST fallback (requires Authorization header)

	http.HandleFunc("/modules", cors(getModulesHandler))
	http.HandleFunc("/uploadModule", cors(uploadModuleHandler))
	http.HandleFunc("/file/", cors(serveFileHandler))

	http.HandleFunc("/addEvaluation", cors(addEvaluationHandler))
	http.HandleFunc("/evaluations/", cors(getEvaluationsHandler))

	http.Handle("/socket.io/", server)

	log.Println("🚀 Server running on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// -------------------- HELPERS --------------------

func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// open CORS for development; change origins in production
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Expose-Headers", "Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

func randomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// parse Authorization header "Bearer <token>"
func tokenFromHeader(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

// -------------------- AUTH Endpoints --------------------

// Register a student (very simple; in production hash passwords)
func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var s Student
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if s.Email == "" || s.Password == "" || s.Name == "" {
		http.Error(w, "name, email and password required", http.StatusBadRequest)
		return
	}
	s.CreatedAt = time.Now()
	_, err := studentsColl.InsertOne(r.Context(), s)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	// create session token
	token, _ := randomToken(32)
	var inserted Student
	_ = studentsColl.FindOne(r.Context(), bson.M{"email": s.Email}).Decode(&inserted)
	sess := Session{
		Token:     token,
		StudentID: inserted.ID,
		CreatedAt: time.Now(),
	}
	_, _ = sessionsColl.InsertOne(r.Context(), sess)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "token": token, "studentId": inserted.ID.Hex(), "name": inserted.Name})
}

// Login: return token
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var cred struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&cred); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	var s Student
	err := studentsColl.FindOne(r.Context(), bson.M{"email": cred.Email, "password": cred.Password}).Decode(&s)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	// create new session token
	token, _ := randomToken(32)
	sess := Session{
		Token:     token,
		StudentID: s.ID,
		CreatedAt: time.Now(),
	}
	_, _ = sessionsColl.InsertOne(r.Context(), sess)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "token": token, "studentId": s.ID.Hex(), "name": s.Name})
}

// Admin: list students (basic)
func listStudentsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	cur, err := studentsColl.Find(ctx, bson.D{})
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)
	var students []Student
	_ = cur.All(ctx, &students)
	json.NewEncoder(w).Encode(students)
}

// -------------------- POSTS (timeline) --------------------

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

func deletePostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "DELETE only", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	_, err = postsColl.DeleteOne(r.Context(), bson.M{"_id": objID})
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// -------------------- CHAT (REST-backed, Socket real-time) --------------------

// get all messages (admin) or by query studentId
func getMessagesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	studentId := r.URL.Query().Get("studentId")
	filter := bson.D{}
	if studentId != "" {
		if obj, err := primitive.ObjectIDFromHex(studentId); err == nil {
			filter = bson.D{{"studentId", obj}}
		}
	}

	cur, err := messagesColl.Find(ctx, filter, options.Find().SetSort(bson.D{{"createdAt", 1}}))
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

// send message via REST: expects Authorization: Bearer <token> to derive student (optional for admin)
func sendMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var incoming struct {
		Sender      string `json:"sender"`
		Receiver    string `json:"receiver"`
		StudentID   string `json:"studentId"`
		StudentName string `json:"studentName"`
		Message     string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	var msg Message
	msg.Sender = incoming.Sender
	msg.Receiver = incoming.Receiver
	msg.Message = incoming.Message
	msg.CreatedAt = time.Now()

	// if studentId provided as hex, parse
	if incoming.StudentID != "" {
		if obj, err := primitive.ObjectIDFromHex(incoming.StudentID); err == nil {
			msg.StudentID = obj
		}
	}
	msg.StudentName = incoming.StudentName

	// persist
	_, err := messagesColl.InsertOne(r.Context(), msg)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// broadcast via socket.io: global + student room if studentId present
	// Note: For REST callers we won't have direct access to server variable here; instead we rely on Socket.IO events for live.
	// But to keep parity, clients should also call socket.emit("send_message", msg) to push real-time. REST still stores the message.

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// -------------------- MODULES (GridFS) --------------------

func uploadModuleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	err := r.ParseMultipartForm(50 << 20) // allow up to ~50MB
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
		http.Error(w, "gridfs error", http.StatusInternalServerError)
		return
	}
	uploadStream, err := bucket.OpenUploadStream(header.Filename)
	if err != nil {
		http.Error(w, "upload stream error", http.StatusInternalServerError)
		return
	}
	defer uploadStream.Close()

	_, err = io.Copy(uploadStream, file)
	if err != nil {
		http.Error(w, "file upload failed", http.StatusInternalServerError)
		return
	}
	fileID := uploadStream.FileID.(primitive.ObjectID)
	fileURL := fmt.Sprintf("%s/file/%s", serverBaseURL(), fileID.Hex())

	module := Module{
		Title:     title,
		FileName:  header.Filename,
		FileURL:   fileURL,
		FileType:  header.Header.Get("Content-Type"),
		CreatedAt: time.Now(),
	}
	_, err = modulesColl.InsertOne(r.Context(), module)
	if err != nil {
		http.Error(w, "db insert error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "fileUrl": fileURL})
}

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

func serveFileHandler(w http.ResponseWriter, r *http.Request) {
	idHex := r.URL.Path[len("/file/"):]
	objID, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.Error(w, "invalid file id", http.StatusBadRequest)
		return
	}
	bucket, _ := gridfs.NewBucket(db)
	stream, err := bucket.OpenDownloadStream(objID)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer stream.Close()
	fileInfo := stream.GetFile()
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", fileInfo.Name))
	// Try to set Content-Type from metadata if present - GridFS store keeps headers, but as fallback use octet-stream
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = io.Copy(w, stream)
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
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func getEvaluationsHandler(w http.ResponseWriter, r *http.Request) {
	studentID := strings.TrimPrefix(r.URL.Path, "/evaluations/")
	if studentID == "" {
		http.Error(w, "studentId missing", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cur, err := evalColl.Find(ctx, bson.M{"studentId": studentID}, options.Find().SetSort(bson.D{{"createdAt", -1}}))
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)
	var evals []Evaluation
	if err := cur.All(ctx, &evals); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(evals)
}

// -------------------- UTIL --------------------

// serverBaseURL tries to generate base url for file links; in production use env var
func serverBaseURL() string {
	base := os.Getenv("PUBLIC_BASE_URL") // set this in env to "https://yourdomain"
	if base != "" {
		return base
	}
	// fallback to railway or localhost with port
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}
	// NOTE: this will often be localhost for dev; set PUBLIC_BASE_URL in production
	return fmt.Sprintf("http://localhost:%s", port)
}
