package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Reactions struct {
	Like int `json:"like"`
	Love int `json:"love"`
	Wow  int `json:"wow"`
	Haha int `json:"haha"`
	Sad  int `json:"sad"`
	Fire int `json:"fire"`
	Clap int `json:"clap"`
}

func (r Reactions) Total() int {
	return r.Like + r.Love + r.Wow + r.Haha + r.Sad + r.Fire + r.Clap
}

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

type Comment struct {
	ID        string    `json:"id"`
	PostID    string    `json:"post_id"`
	Author    string    `json:"author"`
	Content   string    `json:"content"`
	Date      string    `json:"date"`
	CreatedAt time.Time `json:"created_at"`
}

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

type store struct {
	rdb *redis.Client

	mu       sync.RWMutex
	posts    []Post
	comments []Comment
	admin    AdminProfile
	sessions map[string]string
}

var (
	st     *store
	tmpl   *template.Template
	ctx    = context.Background()
	initOnce sync.Once
)

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

func funcMap() template.FuncMap {
	return template.FuncMap{
		"splitLines":    splitLines,
		"reactionTotal": reactionTotal,
		"firstChar":     firstChar,
	}
}

func defaultAdmin() AdminProfile {
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

func (s *store) totalPostViews() int {
	total := 0
	for _, p := range s.posts {
		total += p.Views
	}
	return total
}

func (s *store) totalReactions() int {
	total := 0
	for _, p := range s.posts {
		total += p.Reactions.Total()
	}
	return total
}

func clampReaction(n int) int {
	if n < 0 {
		return 0
	}
	return n
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

func loadAdminFromFile() AdminProfile {
	data, err := os.ReadFile("data/admin.json")
	if err != nil {
		adm := defaultAdmin()
		saveAdminToFile(adm)
		return adm
	}
	var adm AdminProfile
	if err := json.Unmarshal(data, &adm); err != nil || adm.Username == "" {
		adm = defaultAdmin()
		saveAdminToFile(adm)
	}
	return adm
}

func saveAdminToFile(adm AdminProfile) {
	data, _ := json.MarshalIndent(adm, "", "  ")
	os.WriteFile("data/admin.json", data, 0644)
}

func loadPostsFromFile() []Post {
	data, err := os.ReadFile("data/posts.json")
	if err != nil {
		return []Post{}
	}
	var posts []Post
	json.Unmarshal(data, &posts)
	return posts
}

func savePostsToFile(posts []Post) {
	data, _ := json.MarshalIndent(posts, "", "  ")
	os.WriteFile("data/posts.json", data, 0644)
}

func loadCommentsFromFile() []Comment {
	data, err := os.ReadFile("data/comments.json")
	if err != nil {
		return []Comment{}
	}
	var comments []Comment
	json.Unmarshal(data, &comments)
	return comments
}

func saveCommentsToFile(comments []Comment) {
	data, _ := json.MarshalIndent(comments, "", "  ")
	os.WriteFile("data/comments.json", data, 0644)
}

func initStore() {
	st = &store{
		sessions: make(map[string]string),
	}

	kvURL := os.Getenv("KV_URL")
	if kvURL != "" {
		opts, err := redis.ParseURL(kvURL)
		if err != nil {
			log.Printf("Failed to parse KV_URL: %v, falling back to file-based storage", err)
		} else {
			rdb := redis.NewClient(opts)
			if err := rdb.Ping(ctx).Err(); err != nil {
				log.Printf("Failed to connect to Redis/KV: %v, falling back to file-based storage", err)
			} else {
				st.rdb = rdb
				log.Println("Connected to Vercel KV (Redis)")
			}
		}
	}

	st.admin = loadAdminFromFile()
	st.posts = loadPostsFromFile()
	st.comments = loadCommentsFromFile()

	if st.rdb != nil {
		if err := st.loadFromRedis(); err != nil {
			log.Printf("Failed to load data from Redis: %v", err)
		}
	}
}

func (s *store) loadFromRedis() error {
	pipe := s.rdb.Pipeline()
	adminCmd := pipe.Get(ctx, "admin")
	postsCmd := pipe.Get(ctx, "posts")
	commentsCmd := pipe.Get(ctx, "comments")
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return err
	}

	if adminVal, _ := adminCmd.Result(); adminVal != "" {
		var adm AdminProfile
		if err := json.Unmarshal([]byte(adminVal), &adm); err == nil {
			s.admin = adm
		}
	}

	if postsVal, _ := postsCmd.Result(); postsVal != "" {
		var posts []Post
		if err := json.Unmarshal([]byte(postsVal), &posts); err == nil {
			s.posts = posts
		}
	}

	if commentsVal, _ := commentsCmd.Result(); commentsVal != "" {
		var comments []Comment
		if err := json.Unmarshal([]byte(commentsVal), &comments); err == nil {
			s.comments = comments
		}
	}

	return nil
}

func (s *store) syncToRedis() {
	if s.rdb == nil {
		return
	}
	pipe := s.rdb.Pipeline()
	adminData, _ := json.Marshal(s.admin)
	pipe.Set(ctx, "admin", adminData, 0)
	postsData, _ := json.Marshal(s.posts)
	pipe.Set(ctx, "posts", postsData, 0)
	commentsData, _ := json.Marshal(s.comments)
	pipe.Set(ctx, "comments", commentsData, 0)
	pipe.Exec(ctx)
}

func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *store) createSession() string {
	token := generateSessionID()
	s.mu.Lock()
	s.sessions[token] = s.admin.Username
	s.mu.Unlock()
	if s.rdb != nil {
		s.rdb.Set(ctx, "session:"+token, s.admin.Username, 24*time.Hour)
	}
	return token
}

func (s *store) deleteSession(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
	if s.rdb != nil {
		s.rdb.Del(ctx, "session:"+token)
	}
}

func (s *store) isValidSession(token string) bool {
	if token == "" {
		return false
	}
	s.mu.RLock()
	_, ok := s.sessions[token]
	s.mu.RUnlock()
	if ok {
		return true
	}
	if s.rdb != nil {
		val, err := s.rdb.Get(ctx, "session:"+token).Result()
		if err == nil && val != "" {
			s.mu.Lock()
			s.sessions[token] = val
			s.mu.Unlock()
			return true
		}
	}
	return false
}

func getSessionToken(r *http.Request) string {
	c, err := r.Cookie("session")
	if err != nil {
		return ""
	}
	return c.Value
}

func isAdmin(r *http.Request) bool {
	return st.isValidSession(getSessionToken(r))
}

func executeLayout(w http.ResponseWriter, data interface{}, files ...string) {
	tmpl.ExecuteTemplate(w, "layout.html", data)
}

func parseTemplates(files ...string) *template.Template {
	return template.Must(template.New("").Funcs(funcMap()).ParseFiles(files...))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	st.mu.RLock()
	posts := make([]Post, len(st.posts))
	copy(posts, st.posts)
	st.mu.RUnlock()

	executeLayout(w, struct {
		Title   string
		Posts   []Post
		IsAdmin bool
	}{
		Title:   "Home",
		Posts:   posts,
		IsAdmin: isAdmin(r),
	}, "templates/layout.html", "templates/_reactions.html", "templates/index.html")
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	postID := strings.TrimPrefix(r.URL.Path, "/post/")

	st.mu.Lock()
	var currentPost *Post
	for i := range st.posts {
		if st.posts[i].ID == postID {
			currentPost = &st.posts[i]
			currentPost.Views++
			savePostsToFile(st.posts)
			if st.rdb != nil {
				go st.syncToRedis()
			}
			break
		}
	}
	st.mu.Unlock()

	if currentPost == nil {
		http.NotFound(w, r)
		return
	}

	st.mu.RLock()
	var postComments []Comment
	for _, c := range st.comments {
		if c.PostID == postID {
			postComments = append(postComments, c)
		}
	}
	postCopy := *currentPost
	st.mu.RUnlock()

	executeLayout(w, struct {
		Title    string
		Post     Post
		Comments []Comment
		IsAdmin  bool
	}{
		Title:    postCopy.Title,
		Post:     postCopy,
		Comments: postComments,
		IsAdmin:  isAdmin(r),
	}, "templates/layout.html", "templates/post.html")
}

func commentHandler(w http.ResponseWriter, r *http.Request) {
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

	st.mu.Lock()
	st.comments = append(st.comments, comment)
	saveCommentsToFile(st.comments)
	if st.rdb != nil {
		go st.syncToRedis()
	}
	st.mu.Unlock()

	http.Redirect(w, r, "/post/"+postID, http.StatusSeeOther)
}

func reactHandler(w http.ResponseWriter, r *http.Request) {
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

	st.mu.Lock()
	var currentPost *Post
	var postIndex int
	found := false
	for i := range st.posts {
		if st.posts[i].ID == postID {
			currentPost = &st.posts[i]
			postIndex = i
			found = true
			break
		}
	}

	if !found {
		st.mu.Unlock()
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	if !adjustReaction(&currentPost.Reactions, reactType, action) {
		st.mu.Unlock()
		http.Error(w, "Invalid reaction type", http.StatusBadRequest)
		return
	}

	st.posts[postIndex] = *currentPost
	savePostsToFile(st.posts)
	if st.rdb != nil {
		go st.syncToRedis()
	}
	st.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"post_id":    postID,
		"react_type": reactType,
		"action":     action,
		"reactions":  currentPost.Reactions,
	})
}

func adminLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		executeLayout(w, struct {
			Title   string
			Error   string
			IsAdmin bool
		}{
			Title:   "Admin Login",
			Error:   r.URL.Query().Get("error"),
			IsAdmin: isAdmin(r),
		}, "templates/layout.html", "templates/admin-login.html")
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	st.mu.RLock()
	valid := username == st.admin.Username && password == st.admin.Password
	st.mu.RUnlock()

	if valid {
		token := st.createSession()
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400,
		})
		http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/login?error=Invalid+credentials", http.StatusSeeOther)
}

func adminLogoutHandler(w http.ResponseWriter, r *http.Request) {
	token := getSessionToken(r)
	if token != "" {
		st.deleteSession(token)
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(r) {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func adminDashboardHandler(w http.ResponseWriter, r *http.Request) {
	st.mu.RLock()
	posts := make([]Post, len(st.posts))
	copy(posts, st.posts)
	adm := st.admin
	totalViews := st.totalPostViews()
	totalReactions := st.totalReactions()
	totalComments := len(st.comments)
	st.mu.RUnlock()

	executeLayout(w, struct {
		Title          string
		Posts          []Post
		Admin          AdminProfile
		IsAdmin        bool
		TotalViews     int
		TotalReactions int
		TotalComments  int
	}{
		Title:          "Dashboard",
		Posts:          posts,
		Admin:          adm,
		IsAdmin:        true,
		TotalViews:     totalViews,
		TotalReactions: totalReactions,
		TotalComments:  totalComments,
	}, "templates/layout.html", "templates/admin-dashboard.html")
}

func adminProfileHandler(w http.ResponseWriter, r *http.Request) {
	st.mu.RLock()
	adm := st.admin
	postCount := len(st.posts)
	totalViews := st.totalPostViews()
	totalReactions := st.totalReactions()
	totalComments := len(st.comments)
	st.mu.RUnlock()

	executeLayout(w, struct {
		Title          string
		Admin          AdminProfile
		IsAdmin        bool
		PostCount      int
		TotalViews     int
		TotalReactions int
		TotalComments  int
		Success        string
	}{
		Title:          "My Profile",
		Admin:          adm,
		IsAdmin:        true,
		PostCount:      postCount,
		TotalViews:     totalViews,
		TotalReactions: totalReactions,
		TotalComments:  totalComments,
		Success:        r.URL.Query().Get("updated"),
	}, "templates/layout.html", "templates/admin-profile.html")
}

func adminSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		st.mu.RLock()
		adm := st.admin
		st.mu.RUnlock()

		executeLayout(w, struct {
			Title   string
			Admin   AdminProfile
			IsAdmin bool
			Error   string
			Success string
		}{
			Title:   "Admin Settings",
			Admin:   adm,
			IsAdmin: true,
			Error:   r.URL.Query().Get("error"),
			Success: r.URL.Query().Get("success"),
		}, "templates/layout.html", "templates/admin-settings.html")
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

	st.mu.Lock()
	if newPassword != "" {
		if password != st.admin.Password {
			st.mu.Unlock()
			http.Redirect(w, r, "/admin/settings?error=Current+password+is+incorrect", http.StatusSeeOther)
			return
		}
		if len(newPassword) < 4 {
			st.mu.Unlock()
			http.Redirect(w, r, "/admin/settings?error=New+password+must+be+at+least+4+characters", http.StatusSeeOther)
			return
		}
		if newPassword != confirmPassword {
			st.mu.Unlock()
			http.Redirect(w, r, "/admin/settings?error=New+passwords+do+not+match", http.StatusSeeOther)
			return
		}
		st.admin.Password = newPassword
	}

	st.admin.Username = username
	st.admin.DisplayName = strings.TrimSpace(r.FormValue("display_name"))
	st.admin.Email = strings.TrimSpace(r.FormValue("email"))
	st.admin.Bio = strings.TrimSpace(r.FormValue("bio"))
	st.admin.Tagline = strings.TrimSpace(r.FormValue("tagline"))
	st.admin.Location = strings.TrimSpace(r.FormValue("location"))
	st.admin.Website = strings.TrimSpace(r.FormValue("website"))

	file, handler, err := r.FormFile("avatar")
	if err == nil {
		defer file.Close()
		os.MkdirAll("uploads/avatars", 0755)
		ext := filepath.Ext(handler.Filename)
		filename := uuid.New().String() + ext
		dst, errCreate := os.Create("uploads/avatars/" + filename)
		if errCreate == nil {
			defer dst.Close()
			io.Copy(dst, file)
			st.admin.Avatar = "/uploads/avatars/" + filename
		}
	}

	saveAdminToFile(st.admin)
	if st.rdb != nil {
		go st.syncToRedis()
	}
	st.mu.Unlock()

	http.Redirect(w, r, "/admin/profile?updated=1", http.StatusSeeOther)
}

func adminCreateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		st.mu.RLock()
		adm := st.admin
		st.mu.RUnlock()

		executeLayout(w, struct {
			Title   string
			IsAdmin bool
			Admin   AdminProfile
		}{
			Title:   "Create Post",
			IsAdmin: true,
			Admin:   adm,
		}, "templates/layout.html", "templates/admin-create.html")
		return
	}

	r.ParseMultipartForm(10 << 20)

	title := r.FormValue("title")
	content := r.FormValue("content")
	author := r.FormValue("author")

	st.mu.RLock()
	if author == "" {
		author = st.admin.DisplayName
	}
	st.mu.RUnlock()

	var imagePath string
	file, handler, err := r.FormFile("image")
	if err == nil {
		defer file.Close()
		ext := filepath.Ext(handler.Filename)
		filename := uuid.New().String() + ext
		imagePath = "/uploads/images/" + filename
		dst, errCreate := os.Create("uploads/images/" + filename)
		if errCreate == nil {
			defer dst.Close()
			io.Copy(dst, file)
		}
	}

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

	st.mu.Lock()
	st.posts = append([]Post{post}, st.posts...)
	savePostsToFile(st.posts)
	if st.rdb != nil {
		go st.syncToRedis()
	}
	st.mu.Unlock()

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func adminEditHandler(w http.ResponseWriter, r *http.Request) {
	postID := strings.TrimPrefix(r.URL.Path, "/admin/edit/")

	st.mu.Lock()
	var currentPost *Post
	var postIndex int
	for i := range st.posts {
		if st.posts[i].ID == postID {
			currentPost = &st.posts[i]
			postIndex = i
			break
		}
	}

	if currentPost == nil {
		st.mu.Unlock()
		http.NotFound(w, r)
		return
	}

	if r.Method == "GET" {
		postCopy := *currentPost
		st.mu.Unlock()
		executeLayout(w, struct {
			Title   string
			Post    Post
			IsAdmin bool
		}{
			Title:   "Edit Post",
			Post:    postCopy,
			IsAdmin: true,
		}, "templates/layout.html", "templates/admin-edit.html")
		return
	}

	title := r.FormValue("title")
	content := r.FormValue("content")
	author := r.FormValue("author")

	r.ParseMultipartForm(10 << 20)
	file, handler, err := r.FormFile("image")
	if err == nil {
		defer file.Close()
		ext := filepath.Ext(handler.Filename)
		filename := uuid.New().String() + ext
		imagePath := "/uploads/images/" + filename
		dst, errCreate := os.Create("uploads/images/" + filename)
		if errCreate == nil {
			defer dst.Close()
			io.Copy(dst, file)
			currentPost.Image = imagePath
		}
	}

	excerpt := content
	if len(excerpt) > 150 {
		excerpt = excerpt[:150] + "..."
	}

	currentPost.Title = title
	currentPost.Content = content
	currentPost.Excerpt = excerpt
	currentPost.Author = author

	st.posts[postIndex] = *currentPost
	savePostsToFile(st.posts)
	if st.rdb != nil {
		go st.syncToRedis()
	}
	st.mu.Unlock()

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func adminDeleteHandler(w http.ResponseWriter, r *http.Request) {
	postID := strings.TrimPrefix(r.URL.Path, "/admin/delete/")

	st.mu.Lock()
	for i := range st.posts {
		if st.posts[i].ID == postID {
			st.posts = append(st.posts[:i], st.posts[i+1:]...)
			break
		}
	}
	savePostsToFile(st.posts)
	if st.rdb != nil {
		go st.syncToRedis()
	}
	st.mu.Unlock()

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	initOnce.Do(func() {
		tmpl = parseTemplates(
			"templates/layout.html",
			"templates/_reactions.html",
			"templates/admin-create.html",
			"templates/admin-dashboard.html",
			"templates/admin-edit.html",
			"templates/admin-login.html",
			"templates/admin-profile.html",
			"templates/admin-settings.html",
			"templates/index.html",
			"templates/post.html",
		)
		initStore()
	})

	path := r.URL.Path

	if strings.HasPrefix(path, "/static/") || strings.HasPrefix(path, "/uploads/") {
		http.FileServer(http.Dir(".")).ServeHTTP(w, r)
		return
	}

	switch {
	case path == "/admin/login":
		adminLoginHandler(w, r)
	case path == "/admin/logout":
		adminLogoutHandler(w, r)
	case path == "/admin/dashboard":
		authMiddleware(adminDashboardHandler)(w, r)
	case path == "/admin/profile":
		authMiddleware(adminProfileHandler)(w, r)
	case path == "/admin/settings":
		authMiddleware(adminSettingsHandler)(w, r)
	case path == "/admin/create":
		authMiddleware(adminCreateHandler)(w, r)
	case strings.HasPrefix(path, "/admin/edit/"):
		authMiddleware(adminEditHandler)(w, r)
	case strings.HasPrefix(path, "/admin/delete/"):
		authMiddleware(adminDeleteHandler)(w, r)
	case path == "/comment":
		commentHandler(w, r)
	case path == "/post/react":
		reactHandler(w, r)
	case strings.HasPrefix(path, "/post/"):
		postHandler(w, r)
	default:
		homeHandler(w, r)
	}
}
