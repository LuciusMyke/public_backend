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

type Payment struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"_id,omitempty"`
	StudentID     string             `bson:"studentId" json:"studentId"`
	PupilName     string             `bson:"pupilName" json:"pupilName"`
	Age           string             `bson:"age" json:"age"`
	Birthday      string             `bson:"birthday" json:"birthday"`
	Level         string             `bson:"level" json:"level"`
	FatherName    string             `bson:"fatherName" json:"fatherName"`
	FatherJob     string             `bson:"fatherJob" json:"fatherJob"`
	MotherName    string             `bson:"motherName" json:"motherName"`
	MotherJob     string             `bson:"motherJob" json:"motherJob"`
	Address       string             `bson:"address" json:"address"`
	Contact       string             `bson:"contact" json:"contact"`
	Registration  string             `bson:"registration" json:"registration"`
	Miscellaneous string             `bson:"miscellaneous" json:"miscellaneous"`
	Book          string             `bson:"book" json:"book"`
	GraduationFee string             `bson:"graduationFee" json:"graduationFee"`
	Uniform       string             `bson:"uniform" json:"uniform"`
	PEUniform     string             `bson:"peUniform" json:"peUniform"`
	LDUniform     string             `bson:"ldUniform" json:"ldUniform"`
	PTACHair      string             `bson:"ptaChair" json:"ptaChair"`
	Monthly       map[string]string  `bson:"monthly" json:"monthly"`
	GeneralRules  string             `bson:"generalRules" json:"generalRules"`
	CreatedAt     time.Time          `bson:"createdAt" json:"createdAt"`
}

// -------------------- GLOBALS --------------------

var db *mongo.Database
var postsColl, messagesColl, modulesColl, evalColl, paymentsColl *mongo.Collection

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
		log.Fatal("❌ MongoDB connection error:", err)
	}

	db = client.Database(dbName)
	postsColl = db.Collection("posts")
	messagesColl = db.Collection("messages")
	modulesColl = db.Collection("modules")
	evalColl = db.Collection("evaluations")
	paymentsColl = db.Collection("payments")

	log.Println("✅ Connected to MongoDB:", dbName)

	// -------------------- SOCKET.IO --------------------
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
		server.BroadcastToRoom("/", "global", "receive_message", msg)
	})
	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		log.Println("Disconnected:", s.ID(), reason)
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

	http.HandleFunc("/addPayment", cors(addPaymentHandler))
	http.HandleFunc("/getPayments", cors(getPaymentsHandler))

	http.Handle("/socket.io/", server)

	log.Println("🚀 Server running on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// -------------------- CORS --------------------

func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

// -------------------- HANDLERS --------------------

// ✅ POSTS
func getPostsHandler(w http.ResponseWriter, r *http.Request) {
	cur, err := postsColl.Find(r.Context(), bson.M{})
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(r.Context())
	var posts []Post
	cur.All(r.Context(), &posts)
	json.NewEncoder(w).Encode(posts)
}

func uploadPostHandler(w http.ResponseWriter, r *http.Request) {
	var post Post
	if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}
	post.CreatedAt = time.Now()
	_, err := postsColl.InsertOne(r.Context(), post)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ✅ CHAT
func getMessagesHandler(w http.ResponseWriter, r *http.Request) {
	cur, err := messagesColl.Find(r.Context(), bson.M{})
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(r.Context())
	var messages []Message
	cur.All(r.Context(), &messages)
	json.NewEncoder(w).Encode(messages)
}

func sendMessageHandler(w http.ResponseWriter, r *http.Request) {
	var msg Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}
	msg.CreatedAt = time.Now()
	_, err := messagesColl.InsertOne(r.Context(), msg)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ✅ MODULES
func getModulesHandler(w http.ResponseWriter, r *http.Request) {
	cur, err := modulesColl.Find(r.Context(), bson.M{})
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(r.Context())
	var modules []Module
	cur.All(r.Context(), &modules)
	json.NewEncoder(w).Encode(modules)
}

func uploadModuleHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "File error", http.StatusBadRequest)
		return
	}
	defer file.Close()

	bucket, _ := gridfs.NewBucket(db)
	uploadStream, err := bucket.OpenUploadStream(header.Filename)
	if err != nil {
		http.Error(w, "GridFS error", http.StatusInternalServerError)
		return
	}
	defer uploadStream.Close()

	io.Copy(uploadStream, file)

	module := Module{
		Title:     r.FormValue("title"),
		FileName:  header.Filename,
		FileURL:   "/file/" + header.Filename,
		FileType:  header.Header.Get("Content-Type"),
		CreatedAt: time.Now(),
	}
	modulesColl.InsertOne(r.Context(), module)
	json.NewEncoder(w).Encode(module)
}

func serveFileHandler(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Path[len("/file/"):]
	bucket, _ := gridfs.NewBucket(db)
	stream, err := bucket.OpenDownloadStreamByName(filename)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer stream.Close()
	w.Header().Set("Content-Disposition", "inline; filename="+filename)
	io.Copy(w, stream)
}

// ✅ EVALUATIONS
func addEvaluationHandler(w http.ResponseWriter, r *http.Request) {
	var e Evaluation
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}
	e.CreatedAt = time.Now()
	_, err := evalColl.InsertOne(r.Context(), e)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func getEvaluationsHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/evaluations/"):]
	filter := bson.M{}
	if id != "" {
		filter["studentId"] = id
	}
	cur, err := evalColl.Find(r.Context(), filter)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(r.Context())
	var evals []Evaluation
	cur.All(r.Context(), &evals)
	json.NewEncoder(w).Encode(evals)
}

// ✅ PAYMENTS
func addPaymentHandler(w http.ResponseWriter, r *http.Request) {
	var p Payment
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}
	p.CreatedAt = time.Now()
	_, err := paymentsColl.InsertOne(r.Context(), p)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func getPaymentsHandler(w http.ResponseWriter, r *http.Request) {
	cur, err := paymentsColl.Find(r.Context(), bson.M{})
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(r.Context())
	var payments []Payment
	cur.All(r.Context(), &payments)
	json.NewEncoder(w).Encode(payments)
}
