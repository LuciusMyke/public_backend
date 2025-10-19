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
	gradesCollection   *mongo.Collection // ✅ Added for grades
	server             *socketio.Server

	// Map to track connected users
	connectedUsers = make(map[string]socketio.Conn)
)
var BACKEND_URL = "https://publicbackend-production.up.railway.app"

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ No .env file found, using system environment variables")
	}

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Fatal("❌ MONGO_URI not set")
	}

	// Connect to MongoDB
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal("❌ MongoDB connect failed:", err)
	}
	defer client.Disconnect(context.Background())

	if err := client.Ping(context.Background(), nil); err != nil {
		log.Fatal("❌ MongoDB ping failed:", err)
	}
	log.Println("✅ MongoDB connected successfully")

	db := client.Database("admin1")
	usersCollection = db.Collection("users")
	postsCollection = db.Collection("posts")
	messagesCollection = db.Collection("messages")
	paymentCollection = db.Collection("payments")
	modulesCollection = db.Collection("modules")
	gradesCollection = db.Collection("grades") // ✅ Initialize grades collection

	// Setup Socket.IO
	server = socketio.NewServer(nil)

	server.OnConnect("/", func(s socketio.Conn) error {
		log.Println("✅ WebSocket connected:", s.ID())
		return nil
	})

	// Register user UID after connection
	server.OnEvent("/", "register", func(s socketio.Conn, uid string) {
		if uid != "" {
			connectedUsers[uid] = s
			log.Println("Registered UID:", uid, "with socket:", s.ID())
		}
	})

	// Handle disconnection
	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		log.Println("❌ WebSocket disconnected:", reason)
		for uid, conn := range connectedUsers {
			if conn.ID() == s.ID() {
				delete(connectedUsers, uid)
				break
			}
		}
	})

	go server.Serve()
	defer server.Close()

	// Setup Gin
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:1420", "https://publicbackend-production.up.railway.app"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Module struct
type Module struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"_id"`
	Title     string             `bson:"title" json:"title"`
	FileUrl   string             `bson:"fileUrl" json:"fileUrl"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}






	// Socket.IO routes
	r.GET("/socket.io/*any", gin.WrapH(server))
	r.POST("/socket.io/*any", gin.WrapH(server))

	// API routes
	r.POST("/createUser", createUserHandler)
	r.POST("/login", loginHandler)
	r.POST("/user", getUserProfileHandler)
	r.GET("/getUsers", getUsersHandler)
	r.DELETE("/deleteUser", deleteUserHandler)

	r.GET("/getPosts", getPostsHandler)
	r.POST("/uploadPost", uploadPostHandler)
	r.DELETE("/deletePost", deletePostHandler)


	r.GET("/getMessages", getMessagesHandler)
	r.POST("/sendMessage", sendMessageHandler)

	// Payments
	r.POST("/addPayment", addPaymentHandler)
	r.GET("/getPayments", getPaymentsHandler)
	r.PATCH("/updatePaymentMonth", updatePaymentMonthHandler)

	r.POST("/uploadModule", uploadModuleHandler)
	r.GET("/getModules", getModulesHandler)
	r.DELETE("/deleteModule", deleteModuleHandler)

	// ✅ Grades routes
	r.POST("/uploadGrade", uploadGradeHandler)
	r.GET("/getGrades", getGradesHandler)
	r.DELETE("/deleteGrade", deleteGradeHandler)

	
r.Static("/uploads", "./uploads")
	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}
	log.Println("🚀 Server running on port:", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("❌ Server failed:", err)
	}
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

// --- User profile ---
func getUserProfileHandler(c *gin.Context) {
	var req struct {
		UID string `json:"uid"`
	}
	if err := c.BindJSON(&req); err != nil || req.UID == "" {
		c.String(http.StatusBadRequest, "UID required")
		return
	}

	var user bson.M
	if err := usersCollection.FindOne(context.Background(), bson.M{"uid": req.UID}).Decode(&user); err != nil {
		c.String(http.StatusNotFound, "User not found")
		return
	}

	c.JSON(http.StatusOK, user)
}

func getUsersHandler(c *gin.Context) {
	cursor, err := usersCollection.Find(context.Background(), bson.M{})
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch users")
		return
	}
	var users []bson.M
	cursor.All(context.Background(), &users)
	c.JSON(http.StatusOK, users)
}

func deleteUserHandler(c *gin.Context) {
	var req struct {
		UID string `json:"uid"`
	}
	if err := c.BindJSON(&req); err != nil || req.UID == "" {
		c.String(http.StatusBadRequest, "UID required")
		return
	}

	_, err := usersCollection.DeleteOne(context.Background(), bson.M{"uid": req.UID})
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to delete user")
		return
	}

	c.String(http.StatusOK, "User deleted successfully")
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
	cursor, err := messagesCollection.Find(context.Background(), bson.M{})
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch messages")
		return
	}
	var messages []bson.M
	cursor.All(context.Background(), &messages)
	c.JSON(http.StatusOK, messages)
}

func sendMessageHandler(c *gin.Context) {
	var msg map[string]interface{}
	if err := c.BindJSON(&msg); err != nil {
		c.String(http.StatusBadRequest, "Invalid JSON")
		return
	}

	// 🔧 Normalize for both RN and Tauri
	if msg["senderId"] == nil && msg["sender"] != nil {
		msg["senderId"] = msg["sender"]
	}
	if msg["receiverId"] == nil && msg["receiver"] != nil {
		msg["receiverId"] = msg["receiver"]
	}

	msg["createdAt"] = time.Now().Format(time.RFC3339)

	// Save to MongoDB
	_, err := messagesCollection.InsertOne(context.Background(), msg)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to send message")
		return
	}

	senderID := fmt.Sprint(msg["senderId"])
	receiverID := fmt.Sprint(msg["receiverId"])

	// Emit to sender
	if conn, ok := connectedUsers[senderID]; ok {
		conn.Emit("receive_message", msg)
		log.Println("📤 Sent to sender:", senderID)
	}

	// Emit to receiver
	if conn, ok := connectedUsers[receiverID]; ok {
		conn.Emit("receive_message", msg)
		log.Println("📥 Sent to receiver:", receiverID)
	} else {
		log.Println("⚠️ Receiver not connected:", receiverID)
	}

	c.JSON(http.StatusOK, msg)
}

// --- Payments ---
// Enhanced addPayment with monthly default
func addPaymentHandler(c *gin.Context) {
	var payment map[string]interface{}
	if err := c.BindJSON(&payment); err != nil {
		c.String(http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Default monthly structure if not provided
	if _, ok := payment["monthly"].(map[string]interface{}); !ok {
		payment["monthly"] = map[string]string{
			"june": "Pending", "july": "Pending", "august": "Pending",
			"september": "Pending", "october": "Pending", "november": "Pending",
			"december": "Pending", "january": "Pending", "february": "Pending",
			"march": "Pending",
		}
	}

	payment["createdAt"] = time.Now()
	res, err := paymentCollection.InsertOne(context.Background(), payment)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to add payment")
		return
	}

	payment["_id"] = res.InsertedID
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

// PATCH to update a single month
func updatePaymentMonthHandler(c *gin.Context) {
	var req struct {
		ID    string `json:"_id"`
		Month string `json:"month"`
		Value string `json:"value"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid JSON")
		return
	}

	objID, err := primitive.ObjectIDFromHex(req.ID)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid ID")
		return
	}

	update := bson.M{"$set": bson.M{"monthly." + req.Month: req.Value}}
	_, err = paymentCollection.UpdateOne(context.Background(), bson.M{"_id": objID}, update)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to update month")
		return
	}

	c.String(http.StatusOK, "Month updated successfully")
}

func uploadModuleHandler(c *gin.Context) {
	title := c.PostForm("title")

	file, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "File not provided")
		return
	}

	// Ensure uploads folder exists
	if _, err := os.Stat("./uploads"); os.IsNotExist(err) {
		os.Mkdir("./uploads", os.ModePerm)
	}

	// Save file
	filename := filepath.Base(file.Filename)
	savePath := "./uploads/" + filename
	if err := c.SaveUploadedFile(file, savePath); err != nil {
		c.String(http.StatusInternalServerError, "Failed to save file")
		return
	}

	// Generate public URL
	publicUrl := fmt.Sprintf("%s/uploads/%s", BACKEND_URL, filename)

	// Insert module into MongoDB
	module := Module{
		Title:     title,
		FileUrl:   publicUrl,
		CreatedAt: time.Now(),
	}

	res, err := modulesCollection.InsertOne(context.Background(), module)
	if err != nil {
		c.String(http.StatusInternalServerError, "DB insert failed")
		return
	}

	module.ID = res.InsertedID.(primitive.ObjectID)
	c.JSON(http.StatusOK, module)
}

func getModulesHandler(c *gin.Context) {
	cursor, err := modulesCollection.Find(context.Background(), bson.M{})
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch modules")
		return
	}
	var modules []Module
	if err := cursor.All(context.Background(), &modules); err != nil {
		c.String(http.StatusInternalServerError, "Failed to decode modules")
		return
	}
	c.JSON(http.StatusOK, modules)
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
	if id == "" {
		c.String(http.StatusBadRequest, "Module ID required")
		return
	}

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

// ================= Grades Handlers =================
func uploadGradeHandler(c *gin.Context) {
    studentId := c.PostForm("studentId")
    studentEmail := c.PostForm("studentEmail")
    note := c.PostForm("note")

    fileHeader, err := c.FormFile("file")
    filePath := ""
    if err == nil && fileHeader != nil {
        // Save file
        if err := os.MkdirAll("./uploads/grades", os.ModePerm); err != nil {
            c.String(http.StatusInternalServerError, "Failed to create directory")
            return
        }
        filePath = "./uploads/grades/" + fileHeader.Filename
        if err := c.SaveUploadedFile(fileHeader, filePath); err != nil {
            c.String(http.StatusInternalServerError, "Failed to save file")
            return
        }
    }

    grade := map[string]interface{}{
        "studentId":    studentId,
        "studentEmail": studentEmail,
        "note":         note,
        "fileUrl":      filePath,
        "createdAt":    time.Now(),
    }

    res, err := gradesCollection.InsertOne(context.Background(), grade)
    if err != nil {
        c.String(http.StatusInternalServerError, "Failed to add grade")
        return
    }

    grade["_id"] = res.InsertedID
    c.JSON(http.StatusOK, grade)
}


func getGradesHandler(c *gin.Context) {
	cursor, err := gradesCollection.Find(context.Background(), bson.M{})
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch grades")
		return
	}
	var grades []bson.M
	cursor.All(context.Background(), &grades)
	c.JSON(http.StatusOK, grades)
}

func deleteGradeHandler(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		c.String(http.StatusBadRequest, "Grade ID required")
		return
	}

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid grade ID")
		return
	}

	_, err = gradesCollection.DeleteOne(context.Background(), bson.M{"_id": objID})
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to delete grade")
		return
	}

	c.String(http.StatusOK, "Grade deleted successfully")
}
func deletePostHandler(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		c.String(http.StatusBadRequest, "Post ID required")
		return
	}

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid post ID")
		return
	}

	_, err = postsCollection.DeleteOne(context.Background(), bson.M{"_id": objID})
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to delete post")
		return
	}

	c.String(http.StatusOK, "Post deleted successfully")
}
