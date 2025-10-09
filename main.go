//go:generate go get github.com/gin-gonic/gin github.com/gin-contrib/cors github.com/googollee/go-socket.io

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/googollee/go-socket.io"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var client *mongo.Client
var messagesCollection *mongo.Collection
var postsCollection *mongo.Collection
var server *socketio.Server

func main() {
	// Connect to MongoDB
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "your-mongodb-uri-here"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	client, err = mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}

	db := client.Database("chatbot_db")
	messagesCollection = db.Collection("messages")
	postsCollection = db.Collection("posts")

	// Setup Socket.io
	server = socketio.NewServer(nil)

	server.OnConnect("/", func(s socketio.Conn) error {
		s.SetContext("")
		fmt.Println("New user connected:", s.ID())
		return nil
	})

	server.OnEvent("/", "send_message", func(s socketio.Conn, msg map[string]interface{}) {
		fmt.Println("Message received:", msg)
		server.BroadcastToNamespace("/", "receive_message", msg)
	})

	server.OnError("/", func(s socketio.Conn, e error) {
		fmt.Println("Socket error:", e)
	})
	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		fmt.Println("Socket disconnected:", reason)
	})

	go server.Serve()
	defer server.Close()

	// Setup Gin routes
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
		AllowHeaders:     []string{"Origin", "Content-Type"},
		AllowCredentials: true,
	}))

	r.GET("/socket.io/*any", gin.WrapH(server))
	r.POST("/socket.io/*any", gin.WrapH(server))

	r.POST("/sendMessage", sendMessageHandler)
	r.GET("/getMessages", getMessagesHandler)
	r.POST("/uploadPost", uploadPostHandler)
	r.GET("/getPosts", getPostsHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	r.Run(":" + port)
}

func sendMessageHandler(c *gin.Context) {
	var msg bson.M
	if err := c.BindJSON(&msg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	msg["created_at"] = time.Now()
	_, err := messagesCollection.InsertOne(context.Background(), msg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save message"})
		return
	}

	server.BroadcastToNamespace("/", "receive_message", msg)
	c.JSON(http.StatusOK, msg)
}

func getMessagesHandler(c *gin.Context) {
	cur, err := messagesCollection.Find(context.Background(), bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cur.Close(context.Background())

	var messages []bson.M
	if err = cur.All(context.Background(), &messages); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, messages)
}

func uploadPostHandler(c *gin.Context) {
	file, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image provided"})
		return
	}

	imagePath := fmt.Sprintf("./uploads/%s", file.Filename)
	c.SaveUploadedFile(file, imagePath)

	post := bson.M{
		"image":      file.Filename,
		"description": c.PostForm("description"),
		"created_at": time.Now(),
	}
	_, err = postsCollection.InsertOne(context.Background(), post)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload post"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Post uploaded"})
}

func getPostsHandler(c *gin.Context) {
	cur, err := postsCollection.Find(context.Background(), bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cur.Close(context.Background())

	var posts []bson.M
	if err = cur.All(context.Background(), &posts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Prepend full image URLs
	for i := range posts {
		if filename, ok := posts[i]["image"].(string); ok {
			posts[i]["image_url"] = fmt.Sprintf("https://publicbackend-production.up.railway.app/uploads/%s", filename)
		}
	}

	c.JSON(http.StatusOK, posts)
}

