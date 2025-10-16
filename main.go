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

var paymentCollection *mongo.Collection

func main() {
	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️ No .env file found. Using environment variables.")
	}

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Fatal("❌ MONGO_URI not set in environment")
	}

	// Connect to MongoDB
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal("❌ Failed to connect to MongoDB:", err)
	}
	defer client.Disconnect(context.Background())

	db := client.Database("schoolDB")
	paymentCollection = db.Collection("payments")

	// Setup Socket.IO server
	server := socketio.NewServer(nil)

	server.OnConnect("/", func(s socketio.Conn) error {
		log.Println("✅ New WebSocket connection:", s.ID())
		return nil
	})

	server.OnEvent("/", "message", func(s socketio.Conn, msg string) {
		log.Println("💬 Message received:", msg)
		s.Emit("reply", "Message received: "+msg)
	})

	server.OnError("/", func(s socketio.Conn, e error) {
		log.Println("⚠️ WebSocket error:", e)
	})

	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		log.Println("❌ Disconnected:", reason)
	})

	go server.Serve()
	defer server.Close()

	// HTTP Routes
	http.Handle("/socket.io/", server)
	http.HandleFunc("/addPayment", cors(addPaymentHandler))
	http.HandleFunc("/getPayments", cors(getPaymentsHandler))

	// Placeholder routes (optional, so app builds)
	http.HandleFunc("/getPosts", cors(getPostsHandler))
	http.HandleFunc("/uploadPost", cors(uploadPostHandler))
	http.HandleFunc("/getMessages", cors(getMessagesHandler))
	http.HandleFunc("/sendMessage", cors(sendMessageHandler))
	http.HandleFunc("/getModules", cors(getModulesHandler))
	http.HandleFunc("/uploadModule", cors(uploadModuleHandler))
	http.HandleFunc("/file/", cors(serveFileHandler))
	http.HandleFunc("/addEvaluation", cors(addEvaluationHandler))
	http.HandleFunc("/evaluations/", cors(getEvaluationsHandler))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("🚀 Server running on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// === Handlers ===

func addPaymentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
		return
	}
	var payment map[string]interface{}
	json.NewDecoder(r.Body).Decode(&payment)
	payment["createdAt"] = time.Now()

	_, err := paymentCollection.InsertOne(context.Background(), payment)
	if err != nil {
		http.Error(w, "Failed to add payment", http.StatusInternalServerError)
		return
	}
	w.Write([]byte("Payment added successfully"))
}

func getPaymentsHandler(w http.ResponseWriter, r *http.Request) {
	cursor, err := paymentCollection.Find(context.Background(), bson.M{})
	if err != nil {
		http.Error(w, "Failed to fetch payments", http.StatusInternalServerError)
		return
	}
	var payments []bson.M
	cursor.All(context.Background(), &payments)
	json.NewEncoder(w).Encode(payments)
}

// === Placeholder Handlers (so build doesn’t fail) ===

func getPostsHandler(w http.ResponseWriter, r *http.Request)       { w.Write([]byte("getPostsHandler")) }
func uploadPostHandler(w http.ResponseWriter, r *http.Request)     { w.Write([]byte("uploadPostHandler")) }
func getMessagesHandler(w http.ResponseWriter, r *http.Request)    { w.Write([]byte("getMessagesHandler")) }
func sendMessageHandler(w http.ResponseWriter, r *http.Request)    { w.Write([]byte("sendMessageHandler")) }
func getModulesHandler(w http.ResponseWriter, r *http.Request)     { w.Write([]byte("getModulesHandler")) }
func uploadModuleHandler(w http.ResponseWriter, r *http.Request)   { w.Write([]byte("uploadModuleHandler")) }
func serveFileHandler(w http.ResponseWriter, r *http.Request)      { w.Write([]byte("serveFileHandler")) }
func addEvaluationHandler(w http.ResponseWriter, r *http.Request)  { w.Write([]byte("addEvaluationHandler")) }
func getEvaluationsHandler(w http.ResponseWriter, r *http.Request) { w.Write([]byte("getEvaluationsHandler")) }

// === Helper ===

func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			return
		}
		next.ServeHTTP(w, r)
	}
}
