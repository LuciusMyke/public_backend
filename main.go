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
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var paymentCollection *mongo.Collection

func main() {
	// === Load environment ===
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ No .env file found. Using environment variables.")
	}

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Fatal("❌ MONGO_URI not set in environment")
	}

	// === MongoDB connection ===
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal("❌ Failed to connect to MongoDB:", err)
	}
	defer client.Disconnect(context.Background())

	db := client.Database("schoolDB")
	paymentCollection = db.Collection("payments")

	// === Setup Socket.IO ===
	server := socketio.NewServer(nil)

	server.OnConnect("/", func(s socketio.Conn) error {
		log.Println("✅ WebSocket connected:", s.ID())
		return nil
	})

	server.OnEvent("/", "message", func(s socketio.Conn, msg string) {
		log.Println("💬 Message received:", msg)
		s.Emit("reply", "Message received: "+msg)
	})

	server.OnError("/", func(s socketio.Conn, e error) {
		log.Println("⚠️ Socket error:", e)
	})

	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		log.Println("❌ Disconnected:", reason)
	})

	go server.Serve()
	defer server.Close()

	// === Setup Gin ===
	r := gin.Default()

	// ✅ CORS fix
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:1420", "https://publicbackend-production.up.railway.app"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// === Socket.IO routes ===
	r.GET("/socket.io/*any", gin.WrapH(server))
	r.POST("/socket.io/*any", gin.WrapH(server))

	// === Payment routes ===
	r.POST("/addPayment", addPaymentHandler)
	r.GET("/getPayments", getPaymentsHandler)

	// === Placeholder routes ===
	r.GET("/getPosts", getPostsHandler)
	r.POST("/uploadPost", uploadPostHandler)
	r.GET("/getMessages", getMessagesHandler)
	r.POST("/sendMessage", sendMessageHandler)
	r.GET("/getModules", getModulesHandler)
	r.POST("/uploadModule", uploadModuleHandler)
	r.GET("/file/*filepath", serveFileHandler)
	r.POST("/addEvaluation", addEvaluationHandler)
	r.GET("/evaluations/*id", getEvaluationsHandler)

	// === Run server ===
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("🚀 Server running on port:", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("❌ Server failed:", err)
	}
}

// === Handlers ===

func addPaymentHandler(c *gin.Context) {
	var payment map[string]interface{}
	if err := c.BindJSON(&payment); err != nil {
		c.String(http.StatusBadRequest, "Invalid JSON")
		return
	}
	payment["createdAt"] = time.Now()

	_, err := paymentCollection.InsertOne(context.Background(), payment)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to add payment")
		return
	}
	c.String(http.StatusOK, "Payment added successfully")
}

func getPaymentsHandler(c *gin.Context) {
	cursor, err := paymentCollection.Find(context.Background(), bson.M{})
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch payments")
		return
	}
	var payments []bson.M
	cursor.All(context.Background(), &payments)
	c.JSON(http.StatusOK, payments)
}

// === Placeholder Handlers ===

func getPostsHandler(c *gin.Context)       { c.String(http.StatusOK, "getPostsHandler") }
func uploadPostHandler(c *gin.Context)     { c.String(http.StatusOK, "uploadPostHandler") }
func getMessagesHandler(c *gin.Context)    { c.String(http.StatusOK, "getMessagesHandler") }
func sendMessageHandler(c *gin.Context)    { c.String(http.StatusOK, "sendMessageHandler") }
func getModulesHandler(c *gin.Context)     { c.String(http.StatusOK, "getModulesHandler") }
func uploadModuleHandler(c *gin.Context)   { c.String(http.StatusOK, "uploadModuleHandler") }
func serveFileHandler(c *gin.Context)      { c.String(http.StatusOK, "serveFileHandler") }
func addEvaluationHandler(c *gin.Context)  { c.String(http.StatusOK, "addEvaluationHandler") }
func getEvaluationsHandler(c *gin.Context) { c.String(http.StatusOK, "getEvaluationsHandler") }
