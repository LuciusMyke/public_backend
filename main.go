//go:generate go get github.com/gin-gonic/gin github.com/gin-contrib/cors github.com/googollee/go-socket.io
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	socketio "github.com/googollee/go-socket.io"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var postsColl *mongo.Collection
var messagesColl *mongo.Collection

// ✅ MongoDB connection
func initMongo() *mongo.Client {
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("YOUR_MONGODB_URI"))
	if err != nil {
		log.Fatal("Mongo connection error:", err)
	}
	return client
}

// ✅ Data models
type Post struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"_id"`
	Author    string             `bson:"author" json:"user"`
	Content   string             `bson:"content" json:"caption"`
	ImageURL  string             `bson:"imageURL" json:"photoUrl"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

type Message struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"_id"`
	Sender    string             `bson:"sender" json:"sender"`
	Text      string             `bson:"text" json:"message"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

// ✅ Upload a post (from Tauri or React Native)
func uploadPostHandler(c *gin.Context) {
	var req struct {
		User     string `json:"user"`
		Caption  string `json:"caption"`
		PhotoUrl string `json:"photoUrl"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	post := Post{
		Author:    req.User,
		Content:   req.Caption,
		ImageURL:  req.PhotoUrl,
		CreatedAt: time.Now(),
	}

	_, err := postsColl.InsertOne(context.Background(), post)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save post"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "post saved"})
}

// ✅ Fetch all posts
func getPostsHandler(c *gin.Context) {
	cursor, err := postsColl.Find(context.Background(), bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	defer cursor.Close(context.Background())

	var posts []Post
	if err := cursor.All(context.Background(), &posts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parse error"})
		return
	}

	c.JSON(http.StatusOK, posts)
}

// ✅ Chat message handlers
func saveMessage(sender, text string) {
	msg := Message{
		Sender:    sender,
		Text:      text,
		CreatedAt: time.Now(),
	}
	_, _ = messagesColl.InsertOne(context.Background(), msg)
}

func getMessagesHandler(c *gin.Context) {
	cursor, err := messagesColl.Find(context.Background(), bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	defer cursor.Close(context.Background())

	var messages []Message
	if err := cursor.All(context.Background(), &messages); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parse error"})
		return
	}

	c.JSON(http.StatusOK, messages)
}

// ✅ Main function
func main() {
	client := initMongo()
	db := client.Database("school_app")
	postsColl = db.Collection("posts")
	messagesColl = db.Collection("messages")

	r := gin.Default()

	// Enable CORS for mobile + desktop clients
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type"},
		AllowCredentials: true,
	}))

	// ✅ Socket.IO setup
	server := socketio.NewServer(nil)

	server.OnConnect("/", func(s socketio.Conn) error {
		log.Println("Client connected:", s.ID())
		return nil
	})

	server.OnEvent("/", "send_message", func(s socketio.Conn, msg map[string]string) {
		sender := msg["sender"]
		text := msg["message"]

		saveMessage(sender, text)
		server.BroadcastToNamespace("/", "receive_message", msg)
	})

	server.OnError("/", func(s socketio.Conn, e error) {
		log.Println("Socket error:", e)
	})

	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		log.Println("Client disconnected:", reason)
	})

	go server.Serve()
	defer server.Close()

	// ✅ API routes
	r.GET("/posts", getPostsHandler)
	r.POST("/uploadPost", uploadPostHandler)

	r.GET("/getMessages", getMessagesHandler)

	// Attach Socket.IO
	r.GET("/socket.io/*any", gin.WrapH(server))
	r.POST("/socket.io/*any", gin.WrapH(server))

	port := "8080"
	log.Println("🚀 Server running on port", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("Server error:", err)
	}
}

	c.JSON(http.StatusOK, posts)
}


