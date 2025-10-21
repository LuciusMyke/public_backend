//go:generate go get github.com/gin-gonic/gin github.com/gin-contrib/cors github.com/googollee/go-socket.io
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	socketio "github.com/googollee/go-socket.io"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"golang.org/x/crypto/bcrypt"
	"github.com/golang-jwt/jwt/v4"
)

// ====== GLOBALS ======
var (
	usersCollection       *mongo.Collection
	adminUsersCollection  *mongo.Collection
	postsCollection       *mongo.Collection
	messagesCollection    *mongo.Collection
	paymentCollection     *mongo.Collection
	modulesCollection     *mongo.Collection
	gradesCollection      *mongo.Collection
	server                *socketio.Server
	connectedUsers        = make(map[string]socketio.Conn)
	mongoClient           *mongo.Client
	jwtSecret             []byte
)

var BACKEND_URL = "https://publicbackend-production.up.railway.app"

// ===== STRUCTS ======
type Module struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"_id"`
	Title     string             `bson:"title" json:"title"`
	FileUrl   string             `bson:"fileUrl" json:"fileUrl"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

// Admin login/register structs
type creds struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type adminUser struct {
	Username string `bson:"username"`
	Password string `bson:"password"`
}

// ===== MAIN ======
func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ No .env file found")
	}

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Fatal("❌ MONGO_URI not set")
	}
	jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) == 0 {
		log.Fatal("❌ JWT_SECRET not set")
	}

	// MongoDB connect
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal("❌ MongoDB connect failed:", err)
	}
	mongoClient = client
	defer client.Disconnect(context.Background())

	if err := client.Ping(context.Background(), nil); err != nil {
		log.Fatal("❌ MongoDB ping failed:", err)
	}
	log.Println("✅ MongoDB connected successfully")

	db := client.Database("admin1")
	usersCollection = db.Collection("users")
	adminUsersCollection = db.Collection("adminUsers")
	postsCollection = db.Collection("posts")
	messagesCollection = db.Collection("messages")
	paymentCollection = db.Collection("payments")
	modulesCollection = db.Collection("modules")
	gradesCollection = db.Collection("grades")

	// Websocket server
	server = socketio.NewServer(nil)
	server.OnConnect("/", func(s socketio.Conn) error {
		log.Println("✅ WebSocket connected:", s.ID())
		return nil
	})
	server.OnEvent("/", "register", func(s socketio.Conn, uid string) {
		if uid != "" {
			connectedUsers[uid] = s
			log.Println("Registered UID:", uid, "with socket:", s.ID())
		}
	})
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

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:1420", BACKEND_URL},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Socket.IO
	r.GET("/socket.io/*any", gin.WrapH(server))
	r.POST("/socket.io/*any", gin.WrapH(server))

	// ===== ADMIN AUTH =====
	r.POST("/admin/register", adminRegisterHandler)
	r.POST("/admin/login", adminLoginHandler)

	// ===== USER & POST =====
	r.POST("/createUser", createUserHandler)
	r.POST("/login", loginHandler)
	r.POST("/user", getUserProfileHandler)
	r.GET("/getUsers", getUsersHandler)
	r.DELETE("/deleteUser", deleteUserHandler)
	r.GET("/getPosts", getPostsHandler)
	r.POST("/uploadPost", uploadPostHandler)
	r.DELETE("/deletePost", deletePostHandler)

	// ===== CHAT =====
	r.GET("/getMessages", getMessagesHandler)
	r.POST("/sendMessage", sendMessageHandler)

	// ===== PAYMENTS =====
	r.POST("/addPayment", addPaymentHandler)
	r.GET("/getPayments", getPaymentsHandler)
	r.PATCH("/updatePaymentMonth", updatePaymentMonthHandler)

	// ===== MODULES =====
	r.POST("/uploadModule", uploadModuleHandler)
	r.GET("/getModules", getModulesHandler)
	r.DELETE("/deleteModule", deleteModuleHandler)

	// ===== GRADES =====
	r.POST("/uploadGrade", uploadGradeHandler)
	r.GET("/getGrades", getGradesHandler)
	r.DELETE("/deleteGrade", deleteGradeHandler)

	r.Static("/uploads", "./uploads")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}
	log.Println("🚀 Server running on port:", port)
	log.Fatal(r.Run(":" + port))
}

// ==================== ADMIN AUTH HANDLERS ====================
func adminRegisterHandler(c *gin.Context) {
	var creds creds
	if err := c.BindJSON(&creds); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if creds.Username == "" || creds.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing fields"})
		return
	}

	count, err := adminUsersCollection.CountDocuments(context.Background(), bson.M{"username": creds.Username})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "user exists"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(creds.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hash error"})
		return
	}

	_, err = adminUsersCollection.InsertOne(context.Background(), bson.M{
		"username": creds.Username,
		"password": string(hash),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db insert error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "created"})
}

func adminLoginHandler(c *gin.Context) {
	var creds creds
	if err := c.BindJSON(&creds); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	var user adminUser
	if err := adminUsersCollection.FindOne(context.Background(), bson.M{"username": creds.Username}).Decode(&user); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(creds.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": creds.Username,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString(jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": tokenStr})
}

// ==================== USERS & POSTS ====================
func createUserHandler(c *gin.Context) {
	var user struct {
		UID           string `json:"uid"`
		Email         string `json:"email"`
		Name          string `json:"name"`
		Birthday      string `json:"birthday"`
		Age           string `json:"age"`
		Address       string `json:"address"`
		MotherName    string `json:"motherName"`
		FatherName    string `json:"fatherName"`
		MotherOcc     string `json:"motherOcc"`
		FatherOcc     string `json:"fatherOcc"`
		MotherBday    string `json:"motherBday"`
		FatherBday    string `json:"fatherBday"`
		ContactNumber string `json:"contactNumber"`
	}

	if err := c.BindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	doc := bson.M{
		"uid":           user.UID,
		"email":         user.Email,
		"name":          user.Name,
		"birthday":      user.Birthday,
		"age":           user.Age,
		"address":       user.Address,
		"motherName":    user.MotherName,
		"fatherName":    user.FatherName,
		"motherOcc":     user.MotherOcc,
		"fatherOcc":     user.FatherOcc,
		"motherBday":    user.MotherBday,
		"fatherBday":    user.FatherBday,
		"contactNumber": user.ContactNumber,
		"createdAt":     time.Now(),
	}

	_, err := usersCollection.InsertOne(context.Background(), doc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User created successfully"})
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

// ==================== POSTS ====================
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

// ==================== CHAT ====================
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
	msg["createdAt"] = time.Now().Format(time.RFC3339)
	_, err := messagesCollection.InsertOne(context.Background(), msg)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to send message")
		return
	}
	senderID := fmt.Sprint(msg["senderId"])
	receiverID := fmt.Sprint(msg["receiverId"])
	if conn, ok := connectedUsers[senderID]; ok {
		conn.Emit("receive_message", msg)
	}
	if conn, ok := connectedUsers[receiverID]; ok {
		conn.Emit("receive_message", msg)
	}
	c.JSON(http.StatusOK, msg)
}

// ==================== PAYMENTS ====================
func addPaymentHandler(c *gin.Context) {
	var payment map[string]interface{}
	if err := c.BindJSON(&payment); err != nil {
		c.String(http.StatusBadRequest, "Invalid JSON")
		return
	}
	if _, ok := payment["monthly"].(map[string]interface{}); !ok {
		payment["monthly"] = map[string]string{
			"june": "Pending", "july": "Pending", "august": "Pending",
			"september": "Pending", "october": "Pending", "november": "Pending",
			"december": "Pending", "january": "Pending", "february": "Pending", "march": "Pending",
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

func updatePaymentMonthHandler(c *gin.Context) {
	var req struct {
		ID    string `json:"_id"`
		Month string `json:"month"`
		Value string `json:"value"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}
	objID, err := primitive.ObjectIDFromHex(req.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	update := bson.M{"$set": bson.M{"monthly." + req.Month: req.Value}}
	_, err = paymentCollection.UpdateOne(context.Background(), bson.M{"_id": objID}, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update month"})
		return
	}

	var updated bson.M
	err = paymentCollection.FindOne(context.Background(), bson.M{"_id": objID}).Decode(&updated)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch updated payment"})
		return
	}

	for _, conn := range connectedUsers {
		conn.Emit("payment_update", updated)
	}

	c.JSON(http.StatusOK, updated)
}

// ==================== MODULES ====================
func uploadModuleHandler(c *gin.Context) {
	title := c.PostForm("title")
	file, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "File not provided")
		return
	}
	if _, err := os.Stat("./uploads"); os.IsNotExist(err) {
		os.Mkdir("./uploads", os.ModePerm)
	}
	filename := filepath.Base(file.Filename)
	savePath := "./uploads/" + filename
	if err := c.SaveUploadedFile(file, savePath); err != nil {
		c.String(http.StatusInternalServerError, "Failed to save file")
		return
	}
	publicUrl := fmt.Sprintf("%s/uploads/%s", BACKEND_URL, filename)
	module := Module{Title: title, FileUrl: publicUrl, CreatedAt: time.Now()}
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
		_ = os.Remove(filePath)
	}
	_, err = modulesCollection.DeleteOne(context.Background(), bson.M{"_id": objID})
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to delete module")
		return
	}
	c.String(http.StatusOK, "Module deleted successfully")
}

// ==================== GRADES ====================
func uploadGradeHandler(c *gin.Context) {
	var req struct {
		UserID   string `json:"userId"`
		UserName string `json:"userName"`
		PhotoURL string `json:"photoUrl"`
		Note     string `json:"note,omitempty"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}
	doc := bson.M{"userId": req.UserID, "userName": req.UserName, "photoUrl": req.PhotoURL, "note": req.Note, "createdAt": time.Now()}
	res, err := gradesCollection.InsertOne(context.Background(), doc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save grade"})
		return
	}
	doc["_id"] = res.InsertedID
	c.JSON(http.StatusOK, doc)
}

func getGradesHandler(c *gin.Context) {
	userId := c.Query("userId")
	filter := bson.M{}
	if userId != "" {
		filter["userId"] = userId
	}
	cursor, err := gradesCollection.Find(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch grades"})
		return
	}
	var grades []bson.M
	cursor.All(context.Background(), &grades)
	c.JSON(http.StatusOK, grades)
}

func deleteGradeHandler(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Grade ID required"})
		return
	}
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid grade ID"})
		return
	}
	_, err = gradesCollection.DeleteOne(context.Background(), bson.M{"_id": objID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete grade"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Grade deleted successfully"})
}
