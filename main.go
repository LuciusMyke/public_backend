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
	ID        interface{} `bson:"_id,omitempty" json:"_id,omitempty"`
	StudentID string      `bson:"studentId" json:"studentId"`
	Age       string      `bson:"age" json:"age"`

	GrossMotorB int `bson:"grossMotorB" json:"grossMotorB"`
	GrossMotorE int `bson:"grossMotorE" json:"grossMotorE"`
	FineMotorB  int `bson:"fineMotorB" json:"fineMotorB"`
	FineMotorE  int `bson:"fineMotorE" json:"fineMotorE"`
	SelfHelpB   int `bson:"selfHelpB" json:"selfHelpB"`
	SelfHelpE   int `bson:"selfHelpE" json:"selfHelpE"`
	ReceptiveB  int `bson:"receptiveB" json:"receptiveB"`
	ReceptiveE  int `bson:"receptiveE" json:"receptiveE"`




	ExpressiveB int `bson:"expressiveB" json:"expressiveB"`
	ExpressiveE int `bson:"expressiveE" json:"expressiveE"`
	CognitiveB  int `bson:"cognitiveB" json:"cognitiveB"`
	CognitiveE  int `bson:"cognitiveE" json:"cognitiveE"`
	SocialB     int `bson:"socialB" json:"socialB"`
	SocialE     int `bson:"socialE" json:"socialE"`

	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}

type Timeline struct {
	ID        interface{} `bson:"_id,omitempty" json:"_id,omitempty"`
	Title     string      `bson:"title" json:"title"`
	Content   string      `bson:"content" json:"content"`
	Date      time.Time   `bson:"date" json:"date"`
	CreatedAt time.Time   `bson:"createdAt" json:"createdAt"`
}

type Payment struct {
	ID          interface{} `bson:"_id,omitempty" json:"_id,omitempty"`
	StudentName string      `bson:"studentName" json:"studentName"`
	Age         string      `bson:"age" json:"age"`
	Birthday    string      `bson:"birthday" json:"birthday"`
	Level       string      `bson:"level" json:"level"`

	FatherName string `bson:"fatherName" json:"fatherName"`
	FatherJob  string `bson:"fatherJob" json:"fatherJob"`
	Address    string `bson:"address" json:"address"`

	MotherName string `bson:"motherName" json:"motherName"`
	MotherJob  string `bson:"motherJob" json:"motherJob"`
	ContactNo  string `bson:"contactNo" json:"contactNo"`

	Registration float64 `bson:"registration" json:"registration"`
	Miscellaneous float64 `bson:"miscellaneous" json:"miscellaneous"`
	Books         float64 `bson:"books" json:"books"`
	GraduationFee float64 `bson:"graduationFee" json:"graduationFee"`

	Uniform float64 `bson:"uniform" json:"uniform"`
	PE      float64 `bson:"pe" json:"pe"`
	LD      float64 `bson:"ld" json:"ld"`
	PTA     float64 `bson:"pta" json:"pta"`

	Monthly  float64 `bson:"monthly" json:"monthly"`
	June     bool    `bson:"june" json:"june"`
	July     bool    `bson:"july" json:"july"`
	August   bool    `bson:"august" json:"august"`
	September bool   `bson:"september" json:"september"`
	October  bool    `bson:"october" json:"october"`
	November bool    `bson:"november" json:"november"`
	December bool    `bson:"december" json:"december"`
	January  bool    `bson:"january" json:"january"`
	February bool    `bson:"february" json:"february"`
	March    bool    `bson:"march" json:"march"`

	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}

// -------------------- GLOBALS --------------------

var db *mongo.Database
var postsColl, messagesColl, modulesColl, evalColl, timelineColl, paymentColl *mongo.Collection

// -------------------- MAIN --------------------

func main() {
	_ = godotenv.Load()
	mongoURI := os.Getenv("MONGO_URI")
@@ -143,8 +103,6 @@
	messagesColl = db.Collection("messages")
	modulesColl = db.Collection("modules")
	evalColl = db.Collection("evaluations")
	timelineColl = db.Collection("timeline")
	paymentColl = db.Collection("payments")

	log.Println("✅ Connected to MongoDB:", dbName)

@@ -157,7 +115,11 @@
	})
	server.OnEvent("/", "send_message", func(s socketio.Conn, msg Message) {
		msg.CreatedAt = time.Now()
		messagesColl.InsertOne(context.Background(), msg)




		server.BroadcastToRoom("/", "global", "receive_message", msg)
	})
	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
@@ -180,19 +142,14 @@
	http.HandleFunc("/addEvaluation", cors(addEvaluationHandler))
	http.HandleFunc("/evaluations/", cors(getEvaluationsHandler))

	http.HandleFunc("/timeline", cors(getTimelineHandler))
	http.HandleFunc("/addTimeline", cors(addTimelineHandler))

	http.HandleFunc("/addPayment", cors(addPaymentHandler))
	http.HandleFunc("/payments", cors(getPaymentsHandler))

	http.Handle("/socket.io/", server)

	log.Println("🚀 Server running on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// -------------------- CORS --------------------

func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
@@ -205,56 +162,230 @@
	}
}

// -------------------- EXISTING HANDLERS (Posts, Chat, Modules, Evaluations) --------------------
// (Keep your existing working versions — I’ll skip re-pasting them for brevity)

// -------------------- TIMELINE --------------------
func addTimelineHandler(w http.ResponseWriter, r *http.Request) {
















	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var t Timeline
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	t.CreatedAt = time.Now()
	t.Date = time.Now()
	timelineColl.InsertOne(r.Context(), t)



	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func getTimelineHandler(w http.ResponseWriter, r *http.Request) {


	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cur, _ := timelineColl.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{"createdAt", -1}}))
	var list []Timeline
	cur.All(ctx, &list)
	json.NewEncoder(w).Encode(list)
