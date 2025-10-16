//go:generate go get github.com/gin-gonic/gin github.com/gin-contrib/cors github.com/googollee/go-socket.io
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	socketio "github.com/googollee/go-socket.io"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	usersCollection    *mongo.Collection
	postsCollection    *mongo.Collection
	messagesCollection *mongo.Collection
	paymentCollection  *mongo.Collection
	modulesCollection  *mongo.Collection
	server             *socketio.Server
)

func main() {
	// Load .env
	godotenv.Load()

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Fatal("MONGO_URI not set")
	}

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	db := client.Database("admin1")

	// Collections
	usersCollection = db.Collection("users")
	postsCollection = db.Collection("posts")
	messagesCollection = db.Collection("messages")
	paymentCollection = db.Collection("payments")
	modulesCollection = db.Collection("modules")

	// Socket.IO server
	server = socketio.NewServer(nil)
	server.OnConnect("/", func(s socketio.Conn) error {
		log.Println("✅ WebSocket connected:", s.ID())
		return nil
	})

	server.OnEvent("/", "chatMessage", func(s socketio.Conn, msg map[string]string) {
		// msg must include: senderId, receiverId, message
		msg["createdAt"] = time.Now().Format(time.RFC3339)
		_, err := messagesCollection.InsertOne(context.Background(), msg)
		if err != nil {
			log.Println("❌ Failed to save chat message:", err)
			return
		}
		server.BroadcastToNamespace("/", "chatMessage", msg)
	})

	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		log.Println("❌ WebSocket disconnected:", reason)
	})

	go server.Serve()
	defer server.Close()

	// Gin setup
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:1420", "https://publicbackend-production.up.railway.app"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Socket.IO
	r.GET("/socket.io/*any", gin.WrapH(server))
	r.POST("/socket.io/*any", gin.WrapH(server))

	// --- User routes ---
	r.POST("/createUser", createUserHandler)
	r.POST("/login", loginHandler)

	// --- Timeline routes ---
	r.GET("/getPosts", getPostsHandler)
	r.POST("/uploadPost", uploadPostHandler)

	// --- Chat routes ---
	r.GET("/getMessages", getMessagesHandler) // filters by userId if provided
	r.POST("/sendMessage", sendMessageHandler)

	// --- Payments ---
	r.POST("/addPayment", addPaymentHandler)
	r.GET("/getPayments", getPaymentsHandler)

	// --- Modules ---
	r.POST("/uploadModule", uploadModuleHandler)
	r.GET("/getModules", getModulesHandler)
	r.DELETE("/deleteModule", deleteModuleHandler)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}
	log.Println("🚀 Server running on port:", port)
	r.Run(":" + port)
}

// ================= Handlers =================

// --- Users ---
func createUserHandler(c *gin.Context) {
	var user map[string]interface{}
	if err := c.BindJSON(&user); err != nil {
		c.String(http.StatusBadRequest, "Invalid JSON")
		return
	}
	user["createdAt"] = time.Now()
	_, err := usersCollection.InsertOne(context.Background(), user)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to create user")
		return
	}
	c.String(http.StatusOK, "User created successfully")
}

func loginHandler(c *gin.Context) {
	var req map[string]string
	if err := c.BindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid JSON")
		return
	}

	filter := bson.M{"email": req["email"], "password": req["password"]}
	var user map[string]interface{}
	if err := usersCollection.FindOne(context.Background(), filter).Decode(&user); err != nil {
		c.String(http.StatusUnauthorized, "Invalid credentials")
		return
	}
	c.JSON(http.StatusOK, user)
}

// --- Timeline ---
func getPostsHandler(c *gin.Context) {
	cursor, err := postsCollection.Find(context.Background(), bson.M{})
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch posts")
		return
	}
	var posts []bson.M
	cursor.All(context.Background(), &posts)
	c.JSON(http.StatusOK, posts)
}

func uploadPostHandler(c *gin.Context) {
	var post map[string]interface{}
	if err := c.BindJSON(&post); err != nil {
		c.String(http.StatusBadRequest, "Invalid JSON")
		return
	}
	post["createdAt"] = time.Now()
	_, err := postsCollection.InsertOne(context.Background(), post)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to upload post")
		return
	}
	c.JSON(http.StatusOK, post)
}

// --- Chat ---
func getMessagesHandler(c *gin.Context) {
	userId := c.Query("userId")
	filter := bson.M{}
	if userId != "" {
		filter = bson.M{"$or": []bson.M{
			{"senderId": userId},
			{"receiverId": userId},
		}}
	}
	cursor, err := messagesCollection.Find(context.Background(), filter)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch messages")
		return
	}
	var messages []bson.M
	cursor.All(context.Background(), &messages)
	c.JSON(http.StatusOK, messages)
}

func sendMessageHandler(c *gin.Context) {
	var msg map[string]string
	if err := c.BindJSON(&msg); err != nil {
		c.String(http.StatusBadRequest, "Invalid JSON")
		return
	}
	msg["createdAt"] = time.Now().Format(time.RFC3339)
	_, err := messagesCollection.InsertOne(context.Background(), msg)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to send message")
		return
	}
	server.BroadcastToNamespace("/", "chatMessage", msg)
	c.JSON(http.StatusOK, msg)
}

// --- Payments ---
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
	c.JSON(http.StatusOK, payment)
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

// --- Modules ---
func uploadModuleHandler(c *gin.Context) {
	title := c.PostForm("title")
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "File not provided")
		return
	}

	savePath := "./uploads/" + fileHeader.Filename
	os.MkdirAll("./uploads", os.ModePerm)
	if err := c.SaveUploadedFile(fileHeader, savePath); err != nil {
		c.String(http.StatusInternalServerError, "Failed to save file")
		return
	}

	module := map[string]interface{}{
		"title":     title,
		"fileUrl":   savePath,
		"createdAt": time.Now(),
	}

	res, err := modulesCollection.InsertOne(context.Background(), module)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to save module in DB")
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": res.InsertedID, "title": title, "fileUrl": savePath})
}

func getModulesHandler(c *gin.Context) {
	cursor, err := modulesCollection.Find(context.Background(), bson.M{})
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch modules")
		return
	}
	var modules []bson.M
	cursor.All(context.Background(), &modules)
	c.JSON(http.StatusOK, modules)
}

func deleteModuleHandler(c *gin.Context) {
	id := c.Query("id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid module ID")
		return
	}
	var module bson.M
	if err := modulesCollection.FindOne(context.Background(), bson.M{"_id": objID}).Decode(&module); err != nil {
		c.String(http.StatusNotFound, "Module not found")
		return
	}
	if filePath, ok := module["fileUrl"].(string); ok {
		os.Remove(filePath)
	}
	_, err = modulesCollection.DeleteOne(context.Background(), bson.M{"_id": objID})
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to delete module")
		return
	}
	c.String(http.StatusOK, "Module deleted successfully")
}
