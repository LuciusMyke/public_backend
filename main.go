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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
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

// Dynamic Evaluation struct with numeric B/E scores
type Evaluation struct {
	ID        interface{}                                   `bson:"_id,omitempty" json:"_id,omitempty"`
	StudentID string                                        `bson:"studentId" json:"studentId"`
	Age       string                                        `bson:"age" json:"age"`
	Scores    map[string]map[int]struct{ B, E int }        `bson:"scores" json:"scores"`
	CreatedAt time.Time                                     `bson:"createdAt" json:"createdAt"`
}

type Payment struct {
	ID           primitive.ObjectID   `bson:"_id,omitempty" json:"_id,omitempty"`
	StudentID    string               `bson:"studentId" json:"studentId"`
	PupilName    string               `bson:"pupilName" json:"pupilName"`
	Age          string               `bson:"age" json:"age"`
	Birthday     string               `bson:"birthday" json:"birthday"`
	Level        string               `bson:"level" json:"level"`
	FatherName   string               `bson:"fatherName" json:"fatherName"`
	FatherJob    string               `bson:"fatherJob" json:"fatherJob"`
	MotherName   string               `bson:"motherName" json:"motherName"`
	MotherJob    string               `bson:"motherJob" json:"motherJob"`
	Address      string               `bson:"address" json:"address"`
	Contact      string               `bson:"contact" json:"contact"`
	Registration string               `bson:"registration" json:"registration"`
	Miscellaneous string              `bson:"miscellaneous" json:"miscellaneous"`
	Book         string               `bson:"book" json:"book"`
	GraduationFee string              `bson:"graduationFee" json:"graduationFee"`
	Uniform      string               `bson:"uniform" json:"uniform"`
	PEUniform    string               `bson:"peUniform" json:"peUniform"`
	LDUniform    string               `bson:"ldUniform" json:"ldUniform"`
	PTACHair     string               `bson:"ptaChair" json:"ptaChair"`
	Monthly      map[string]string    `bson:"monthly" json:"monthly"`
	GeneralRules string               `bson:"generalRules" json:"generalRules"`
	CreatedAt    time.Time            `bson:"createdAt" json:"createdAt"`
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
		log.Fatal(err)
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
	// POSTS
	http.HandleFunc("/posts", cors(getPostsHandler))
	http.HandleFunc("/uploadPost", cors(uploadPostHandler))
	// CHAT
	http.HandleFunc("/getMessages", cors(getMessagesHandler))
	http.HandleFunc("/sendMessage", cors(sendMessageHandler))
	// MODULES
	http.HandleFunc("/modules", cors(getModulesHandler))
	http.HandleFunc("/uploadModule", cors(uploadModuleHandler))
	http.HandleFunc("/file/", cors(serveFileHandler))
	// EVALUATIONS
	http.HandleFunc("/addEvaluation", cors(addEvaluationHandler))
	http.HandleFunc("/evaluations", cors(getEvaluationsHandler))
	// PAYMENTS
	http.HandleFunc("/addPayment", cors(addPaymentHandler))
	http.HandleFunc("/getPayments", cors(getPaymentsHandler))
	// SOCKET.IO
	http.Handle("/socket.io/", server)

	log.Println("🚀 Server running on port:", port)
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

// -------------------- PAYMENTS --------------------

func addPaymentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var p Payment
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	p.CreatedAt = time.Now()
	if p.StudentID == "" {
		p.StudentID = p.PupilName
	}
	_, err := paymentsColl.InsertOne(r.Context(), p)
	if err != nil {
		http.Error(w, "DB insert error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func getPaymentsHandler(w http.ResponseWriter, r *http.Request) {
	studentID := r.URL.Query().Get("studentId")
	filter := bson.M{}
	if studentID != "" {
		filter["studentId"] = studentID
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cur, err := paymentsColl.Find(ctx, filter)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)

	var payments []Payment
	if err := cur.All(ctx, &payments); err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(payments)
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
	if eval.Scores == nil {
		eval.Scores = make(map[string]map[int]struct{ B, E int })
	}

	_, err := evalColl.InsertOne(r.Context(), eval)
	if err != nil {
		http.Error(w, "DB insert error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func getEvaluationsHandler(w http.ResponseWriter, r *http.Request) {
	studentID := r.URL.Query().Get("studentId")
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

// -------------------- Placeholder Handlers --------------------
// Implement these as needed in your existing code

func getPostsHandler(w http.ResponseWriter, r *http.Request)            {}
func uploadPostHandler(w http.ResponseWriter, r *http.Request)        {}
func getMessagesHandler(w http.ResponseWriter, r *http.Request)       {}
func sendMessageHandler(w http.ResponseWriter, r *http.Request)       {}
func getModulesHandler(w http.ResponseWriter, r *http.Request)        {}
func uploadModuleHandler(w http.ResponseWriter, r *http.Request)      {}
func serveFileHandler(w http.ResponseWriter, r *http.Request)         {}
