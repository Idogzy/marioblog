package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Reactions structure
type Reactions struct {
	Like int `json:"like"`
	Love int `json:"love"`
	Wow  int `json:"wow"`
	Haha int `json:"haha"`
	Sad  int `json:"sad"`
	Fire int `json:"fire"`
	Clap int `json:"clap"`
}

// Total returns the sum of all reactions
func (r Reactions) Total() int {
	return r.Like + r.Love + r.Wow + r.Haha + r.Sad + r.Fire + r.Clap
}

// Post structure
type Post struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Excerpt   string    `json:"excerpt"`
	Image     string    `json:"image"`
	Author    string    `json:"author"`
	Date      string    `json:"date"`
	Views     int       `json:"views"`
	CreatedAt time.Time `json:"created_at"`
	Reactions Reactions `json:"reactions"`
}

// Comment structure
type Comment struct {
	ID        string    `json:"id"`
	PostID    string    `json:"post_id"`
	Author    string    `json:"author"`
	Content   string    `json:"content"`
	Date      string    `json:"date"`
	CreatedAt time.Time `json:"created_at"`
}

// AdminProfile structure
type AdminProfile struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Bio         string `json:"bio"`
	Tagline     string `json:"tagline"`
	Avatar      string `json:"avatar"`
	Location    string `json:"location"`
	Website     string `json:"website"`
	JoinedDate  string `json:"joined_date"`
}

// App struct
type App struct {
	posts   []Post
	comments []Comment
	admin   AdminProfile
}

// Global variables
var (
	app           *App
	adminLoggedIn bool
)

// Helper function for splitting lines in template
func splitLines(content string) []string {
	return strings.Split(content, "\n")
}

func reactionTotal(r Reactions) int {
	return r.Total()
}

func firstChar(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "?"
	}
	return strings.ToUpper(string([]rune(s)[0]))
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"splitLines":    splitLines,
		"reactionTotal": reactionTotal,
		"firstChar":     firstChar,
	}
}

func parseTemplates(files ...string) *template.Template {
	return template.Must(template.New("").Funcs(templateFuncs()).ParseFiles(files...))
}

func executeLayout(w http.ResponseWriter, data interface{}, files ...string) {
	if err := parseTemplates(files...).ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func main() {
	// Create directories
	os.MkdirAll("data", 0755)
	os.MkdirAll("uploads/images", 0755)
	os.MkdirAll("uploads/avatars", 0755)
	os.MkdirAll("static", 0755)
	os.MkdirAll("templates", 0755)

	// Initialize app
	app = &App{
		posts:    []Post{},
		comments: []Comment{},
	}

	// Load existing data
	app.loadAdmin()
	app.loadPosts()
	app.loadComments()

	// Add sample post if no posts exist
	if len(app.posts) == 0 {
		app.addSamplePost()
	}

	// Serve static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	http.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("./uploads"))))

	// Public routes
	http.HandleFunc("/", app.homeHandler)
	http.HandleFunc("/post/", app.postHandler)
	http.HandleFunc("/comment", app.commentHandler)
	http.HandleFunc("/post/react", app.reactHandler)

	// Admin routes
	http.HandleFunc("/admin/login", app.adminLoginHandler)
	http.HandleFunc("/admin/logout", app.adminLogoutHandler)
	http.HandleFunc("/admin/dashboard", app.authMiddleware(app.adminDashboardHandler))
	http.HandleFunc("/admin/profile", app.authMiddleware(app.adminProfileHandler))
	http.HandleFunc("/admin/settings", app.authMiddleware(app.adminSettingsHandler))
	http.HandleFunc("/admin/create", app.authMiddleware(app.adminCreateHandler))
	http.HandleFunc("/admin/edit/", app.authMiddleware(app.adminEditHandler))
	http.HandleFunc("/admin/delete/", app.authMiddleware(app.adminDeleteHandler))

	fmt.Println("Server running at http://localhost:8080")
	fmt.Println("Admin login: http://localhost:8080/admin/login")
	fmt.Printf("Username: %s\n", app.admin.Username)
	http.ListenAndServe(":8080", nil)
}

// Load posts from JSON file
func (a *App) loadPosts() {
	data, err := os.ReadFile("data/posts.json")
	if err != nil {
		return
	}
	json.Unmarshal(data, &a.posts)
}

// Save posts to JSON file
func (a *App) savePosts() {
	data, _ := json.MarshalIndent(a.posts, "", "  ")
	os.WriteFile("data/posts.json", data, 0644)
}

// Load comments from JSON file
func (a *App) loadComments() {
	data, err := os.ReadFile("data/comments.json")
	if err != nil {
		return
	}
	json.Unmarshal(data, &a.comments)
}

// Save comments to JSON file
func (a *App) saveComments() {
	data, _ := json.MarshalIndent(a.comments, "", "  ")
	os.WriteFile("data/comments.json", data, 0644)
}

func defaultAdminProfile() AdminProfile {
	return AdminProfile{
		Username:    "Mariomini",
		Password:    "rackyjan",
		DisplayName: "John Idoghor",
		Email:       "storyteller@example.com",
		Bio:         "Writer of tales, keeper of memories, and chronicler of life's beautiful mess.",
		Tagline:     "Where stories come alive, one word at a time",
		Location:    "Nigeria",
		JoinedDate:  "May 2024",
	}
}

func (a *App) loadAdmin() {
	data, err := os.ReadFile("data/admin.json")
	if err != nil {
		a.admin = defaultAdminProfile()
		a.saveAdmin()
		return
	}
	if err := json.Unmarshal(data, &a.admin); err != nil || a.admin.Username == "" {
		a.admin = defaultAdminProfile()
		a.saveAdmin()
	}
}

func (a *App) saveAdmin() {
	data, _ := json.MarshalIndent(a.admin, "", "  ")
	os.WriteFile("data/admin.json", data, 0644)
}

func (a *App) totalPostViews() int {
	total := 0
	for _, p := range a.posts {
		total += p.Views
	}
	return total
}

func (a *App) totalReactions() int {
	total := 0
	for _, p := range a.posts {
		total += p.Reactions.Total()
	}
	return total
}

// Add sample post
func (a *App) addSamplePost() {
	samplePost := Post{
		ID:        uuid.New().String(),
		Title:     "The Whispering Woods",
		Content:   "Once upon a time, deep in the heart of an ancient forest, there stood trees that had witnessed centuries of secrets. The locals called it the Whispering Woods, for they believed the wind carried messages from ancestors through the rustling leaves.\n\nOne day, a young storyteller named Elara ventured into these woods, seeking inspiration for her next tale. As she walked beneath the canopy of intertwined branches, she heard a soft murmur.\n\n'Listen,' the wind seemed to say, 'for every leaf holds a story.'\n\nAnd so Elara sat beneath the oldest oak, opened her journal, and began to write. The words flowed like a river, tales of love, loss, courage, and hope.\n\nFrom that day forward, she returned every week, and each time, the woods shared new secrets.\n\nThis blog is my Whispering Woods, a place where stories come alive.",
		Excerpt:   "Deep in an ancient forest where trees whisper secrets, a storyteller discovers the magic of tales waiting to be told...",
		Image:     "",
		Author:    "The Storyteller",
		Date:      time.Now().Format("January 2, 2006"),
		Views:     0,
		CreatedAt: time.Now(),
	}
	a.posts = append(a.posts, samplePost)
	a.savePosts()
}

// Home page handler
func (a *App) homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data := struct {
		Title   string
		Posts   []Post
		IsAdmin bool
	}{
		Title:   "Home",
		Posts:   a.posts,
		IsAdmin: adminLoggedIn,
	}

	executeLayout(w, data, "templates/layout.html", "templates/_reactions.html", "templates/index.html")
}

// Single post handler
func (a *App) postHandler(w http.ResponseWriter, r *http.Request) {
	postID := strings.TrimPrefix(r.URL.Path, "/post/")

	var currentPost *Post
	for i, post := range a.posts {
		if post.ID == postID {
			currentPost = &a.posts[i]
			currentPost.Views++
			a.savePosts()
			break
		}
	}

	if currentPost == nil {
		http.NotFound(w, r)
		return
	}

	// Get comments for this post
	var postComments []Comment
	for _, comment := range a.comments {
		if comment.PostID == postID {
			postComments = append(postComments, comment)
		}
	}

	data := struct {
		Title    string
		Post     Post
		Comments []Comment
		IsAdmin  bool
	}{
		Title:    currentPost.Title,
		Post:     *currentPost,
		Comments: postComments,
		IsAdmin:  adminLoggedIn,
	}

	executeLayout(w, data, "templates/layout.html", "templates/post.html")
}

// Comment handler
func (a *App) commentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	postID := r.FormValue("post_id")
	author := r.FormValue("author")
	content := r.FormValue("content")

	if author == "" || content == "" {
		http.Redirect(w, r, "/post/"+postID, http.StatusSeeOther)
		return
	}

	comment := Comment{
		ID:        uuid.New().String(),
		PostID:    postID,
		Author:    author,
		Content:   content,
		Date:      time.Now().Format("January 2, 2006"),
		CreatedAt: time.Now(),
	}

	a.comments = append(a.comments, comment)
	a.saveComments()

	http.Redirect(w, r, "/post/"+postID, http.StatusSeeOther)
}

// React handler
func (a *App) reactHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	postID := r.FormValue("post_id")
	reactType := r.FormValue("type")
	action := r.FormValue("action")
	if action == "" {
		action = "add"
	}
	if action != "add" && action != "remove" {
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	if postID == "" || reactType == "" {
		http.Error(w, "Missing post_id or type", http.StatusBadRequest)
		return
	}

	var currentPost *Post
	var postIndex int
	found := false
	for i, post := range a.posts {
		if post.ID == postID {
			currentPost = &a.posts[i]
			postIndex = i
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	if !adjustReaction(&currentPost.Reactions, reactType, action) {
		http.Error(w, "Invalid reaction type", http.StatusBadRequest)
		return
	}

	// Save posts
	a.posts[postIndex] = *currentPost
	a.savePosts()

	// Respond with JSON
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success":    true,
		"post_id":    postID,
		"react_type": reactType,
		"action":     action,
		"reactions":  currentPost.Reactions,
	}
	json.NewEncoder(w).Encode(response)
}

func adjustReaction(r *Reactions, reactType, action string) bool {
	delta := 1
	if action == "remove" {
		delta = -1
	}
	switch reactType {
	case "like":
		r.Like = clampReaction(r.Like + delta)
	case "love":
		r.Love = clampReaction(r.Love + delta)
	case "wow":
		r.Wow = clampReaction(r.Wow + delta)
	case "haha":
		r.Haha = clampReaction(r.Haha + delta)
	case "sad":
		r.Sad = clampReaction(r.Sad + delta)
	case "fire":
		r.Fire = clampReaction(r.Fire + delta)
	case "clap":
		r.Clap = clampReaction(r.Clap + delta)
	default:
		return false
	}
	return true
}

func clampReaction(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// Admin login handler
func (a *App) adminLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		data := struct {
			Title   string
			Error   string
			IsAdmin bool
		}{
			Title:   "Admin Login",
			Error:   r.URL.Query().Get("error"),
			IsAdmin: adminLoggedIn,
		}
		executeLayout(w, data, "templates/layout.html", "templates/admin-login.html")
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	if username == a.admin.Username && password == a.admin.Password {
		adminLoggedIn = true
		http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/login?error=Invalid+credentials", http.StatusSeeOther)
}

// Admin logout handler
func (a *App) adminLogoutHandler(w http.ResponseWriter, r *http.Request) {
	adminLoggedIn = false
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Auth middleware
func (a *App) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !adminLoggedIn {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// Admin dashboard handler
func (a *App) adminDashboardHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title           string
		Posts           []Post
		Admin           AdminProfile
		IsAdmin         bool
		TotalViews      int
		TotalReactions  int
		TotalComments   int
	}{
		Title:          "Dashboard",
		Posts:          a.posts,
		Admin:          a.admin,
		IsAdmin:        adminLoggedIn,
		TotalViews:     a.totalPostViews(),
		TotalReactions: a.totalReactions(),
		TotalComments:  len(a.comments),
	}

	executeLayout(w, data, "templates/layout.html", "templates/admin-dashboard.html")
}

func (a *App) adminProfileHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title           string
		Admin           AdminProfile
		IsAdmin         bool
		PostCount       int
		TotalViews      int
		TotalReactions  int
		TotalComments   int
		Success         string
	}{
		Title:          "My Profile",
		Admin:          a.admin,
		IsAdmin:        adminLoggedIn,
		PostCount:      len(a.posts),
		TotalViews:     a.totalPostViews(),
		TotalReactions: a.totalReactions(),
		TotalComments:  len(a.comments),
		Success:        r.URL.Query().Get("updated"),
	}

	executeLayout(w, data, "templates/layout.html", "templates/admin-profile.html")
}

func (a *App) adminSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		data := struct {
			Title   string
			Admin   AdminProfile
			IsAdmin bool
			Error   string
			Success string
		}{
			Title:   "Admin Settings",
			Admin:   a.admin,
			IsAdmin: adminLoggedIn,
			Error:   r.URL.Query().Get("error"),
			Success: r.URL.Query().Get("success"),
		}
		executeLayout(w, data, "templates/layout.html", "templates/admin-settings.html")
		return
	}

	r.ParseMultipartForm(10 << 20)

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if username == "" {
		http.Redirect(w, r, "/admin/settings?error=Username+is+required", http.StatusSeeOther)
		return
	}

	if newPassword != "" {
		if password != a.admin.Password {
			http.Redirect(w, r, "/admin/settings?error=Current+password+is+incorrect", http.StatusSeeOther)
			return
		}
		if len(newPassword) < 4 {
			http.Redirect(w, r, "/admin/settings?error=New+password+must+be+at+least+4+characters", http.StatusSeeOther)
			return
		}
		if newPassword != confirmPassword {
			http.Redirect(w, r, "/admin/settings?error=New+passwords+do+not+match", http.StatusSeeOther)
			return
		}
		a.admin.Password = newPassword
	}

	a.admin.Username = username
	a.admin.DisplayName = strings.TrimSpace(r.FormValue("display_name"))
	a.admin.Email = strings.TrimSpace(r.FormValue("email"))
	a.admin.Bio = strings.TrimSpace(r.FormValue("bio"))
	a.admin.Tagline = strings.TrimSpace(r.FormValue("tagline"))
	a.admin.Location = strings.TrimSpace(r.FormValue("location"))
	a.admin.Website = strings.TrimSpace(r.FormValue("website"))

	file, handler, err := r.FormFile("avatar")
	if err == nil {
		defer file.Close()
		os.MkdirAll("uploads/avatars", 0755)
		ext := filepath.Ext(handler.Filename)
		filename := uuid.New().String() + ext
		dst, _ := os.Create("uploads/avatars/" + filename)
		defer dst.Close()
		io.Copy(dst, file)
		a.admin.Avatar = "/uploads/avatars/" + filename
	}

	a.saveAdmin()
	http.Redirect(w, r, "/admin/profile?updated=1", http.StatusSeeOther)
}

// Admin create post handler
func (a *App) adminCreateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		data := struct {
			Title   string
			IsAdmin bool
			Admin   AdminProfile
		}{
			Title:   "Create Post",
			IsAdmin: adminLoggedIn,
			Admin:   a.admin,
		}
		executeLayout(w, data, "templates/layout.html", "templates/admin-create.html")
		return
	}

	// Parse form
	r.ParseMultipartForm(10 << 20)

	title := r.FormValue("title")
	content := r.FormValue("content")
	author := r.FormValue("author")
	if author == "" {
		author = a.admin.DisplayName
	}

	// Handle image upload
	var imagePath string
	file, handler, err := r.FormFile("image")
	if err == nil {
		defer file.Close()
		ext := filepath.Ext(handler.Filename)
		filename := uuid.New().String() + ext
		imagePath = "/uploads/images/" + filename

		dst, _ := os.Create("uploads/images/" + filename)
		defer dst.Close()
		io.Copy(dst, file)
	}

	// Create excerpt
	excerpt := content
	if len(excerpt) > 150 {
		excerpt = excerpt[:150] + "..."
	}

	post := Post{
		ID:        uuid.New().String(),
		Title:     title,
		Content:   content,
		Excerpt:   excerpt,
		Image:     imagePath,
		Author:    author,
		Date:      time.Now().Format("January 2, 2006"),
		Views:     0,
		CreatedAt: time.Now(),
	}

	a.posts = append([]Post{post}, a.posts...)
	a.savePosts()

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

// Admin edit post handler
func (a *App) adminEditHandler(w http.ResponseWriter, r *http.Request) {
	postID := strings.TrimPrefix(r.URL.Path, "/admin/edit/")

	var currentPost *Post
	var postIndex int
	for i, post := range a.posts {
		if post.ID == postID {
			currentPost = &a.posts[i]
			postIndex = i
			break
		}
	}

	if currentPost == nil {
		http.NotFound(w, r)
		return
	}

	if r.Method == "GET" {
		data := struct {
			Title   string
			Post    Post
			IsAdmin bool
		}{
			Title:   "Edit Post",
			Post:    *currentPost,
			IsAdmin: adminLoggedIn,
		}
		executeLayout(w, data, "templates/layout.html", "templates/admin-edit.html")
		return
	}

	// Update post
	title := r.FormValue("title")
	content := r.FormValue("content")
	author := r.FormValue("author")

	// Handle image upload
	r.ParseMultipartForm(10 << 20)
	file, handler, err := r.FormFile("image")
	if err == nil {
		defer file.Close()
		ext := filepath.Ext(handler.Filename)
		filename := uuid.New().String() + ext
		imagePath := "/uploads/images/" + filename

		dst, _ := os.Create("uploads/images/" + filename)
		defer dst.Close()
		io.Copy(dst, file)

		currentPost.Image = imagePath
	}

	excerpt := content
	if len(excerpt) > 150 {
		excerpt = excerpt[:150] + "..."
	}

	currentPost.Title = title
	currentPost.Content = content
	currentPost.Excerpt = excerpt
	currentPost.Author = author

	a.posts[postIndex] = *currentPost
	a.savePosts()

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

// Admin delete post handler
func (a *App) adminDeleteHandler(w http.ResponseWriter, r *http.Request) {
	postID := strings.TrimPrefix(r.URL.Path, "/admin/delete/")

	for i, post := range a.posts {
		if post.ID == postID {
			a.posts = append(a.posts[:i], a.posts[i+1:]...)
			break
		}
	}

	a.savePosts()
	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}