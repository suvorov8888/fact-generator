package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	log.Println("Запускаем сервер на :8080...")

	// Используем ServeMux для маршрутизации
	mux := http.NewServeMux()

	// Отдаём HTML-файл из папки templates
	mux.HandleFunc("/", serveTemplate)

	// Наш будущий обработчик для генерации фактов
	mux.HandleFunc("/generate-fact", generateFactHandler)

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Ошибка запуска сервера: %s\n", err)
	}
}

// Обработчик для отдачи HTML-шаблона
func serveTemplate(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

// Структура для парсинга JSON-запроса от клиента
type TopicRequest struct {
	Topic string `json:"topic"`
}

func generateFactHandler(w http.ResponseWriter, r *http.Request) {
	// Проверяем, что метод запроса - POST
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не разрешен", http.StatusMethodNotAllowed)
		return
	}

	// Читаем тему из тела запроса
	var topicReq TopicRequest
	if err := json.NewDecoder(r.Body).Decode(&topicReq); err != nil {
		http.Error(w, "Ошибка чтения запроса: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 1. Подготовка запроса к API
	apiURL := "https://openrouter.ai/api/v1/chat/completions"
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		http.Error(w, "API ключ не найден", http.StatusInternalServerError)
		return
	}

	// Формируем промпт для AI
	prompt := "Сгенерируй случайный интересный факт на русском языке. Ответ должен содержать только сам факт, без вступлений и объяснений."
	if topicReq.Topic != "" {
		// Если тема указана, добавляем ее в промпт
		prompt = "Сгенерируй интересный факт на тему '" + topicReq.Topic + "' на русском языке. Ответ должен содержать только сам факт, без вступлений и объяснений."
	}

	// Вместо структур используем map[string]interface{} для простоты
	requestBody := map[string]interface{}{
		"model": "deepseek/deepseek-chat",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		http.Error(w, "Ошибка при подготовке запроса: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Создание и отправка запроса
	// Используем http.NewRequest для добавления заголовков
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		http.Error(w, "Ошибка при создании запроса к API: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Add("Authorization", "Bearer "+apiKey)
	req.Header.Add("Content-Type", "application/json")

	// Используем стандартный HTTP-клиент для выполнения запроса
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "Ошибка при отправке запроса к API: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// 3. Обработка ответа от API. Используем io.ReadAll вместо устаревшего ioutil.ReadAll
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Ошибка при чтении ответа: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Парсим JSON в map[string]interface{}
	var apiResponse map[string]interface{}
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		http.Error(w, "Ошибка при парсинге JSON ответа: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Проверяем, не вернул ли API ошибку
	if errData, ok := apiResponse["error"].(map[string]interface{}); ok {
		if errMsg, ok := errData["message"].(string); ok {
			log.Printf("API Error: %s", errMsg)
			http.Error(w, "Ошибка от API: "+errMsg, http.StatusInternalServerError)
			return
		}
	}
	// 4. Отправка результата клиенту (браузеру)
	// Достаем результат из вложенных map
	choices, ok := apiResponse["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		// Логируем тело ответа для отладки
		log.Printf("API response body: %s", string(body))
		http.Error(w, "Не удалось получить факт: неверный формат ответа.", http.StatusInternalServerError)
		return
	}

	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		http.Error(w, "Не удалось получить факт: неверный формат choice.", http.StatusInternalServerError)
		return
	}

	message, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		http.Error(w, "Не удалось получить факт: неверный формат message.", http.StatusInternalServerError)
		return
	}

	content, ok := message["content"].(string)
	if !ok {
		http.Error(w, "Не удалось получить факт: неверный формат content.", http.StatusInternalServerError)
		return
	}

	w.Write([]byte(content))
}
