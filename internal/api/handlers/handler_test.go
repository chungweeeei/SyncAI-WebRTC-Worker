package handlers_test

import (
	"SyncAI-WebRTC-Worker/internal/api/handlers"
	"SyncAI-WebRTC-Worker/internal/config"
	rtc "SyncAI-WebRTC-Worker/internal/webrtc"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockSessionManager struct {
	mock.Mock
}

func (m *MockSessionManager) CreateSession(ctx context.Context, offerSDP string, opts rtc.SessionOptions) (string, string, error) {
	args := m.Called(ctx, offerSDP, opts)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *MockSessionManager) DeleteSession(sessionID string) error {
	args := m.Called(sessionID)
	return args.Error(0)
}

func (m *MockSessionManager) CreateAudioSession(offerSDP string, opts rtc.AudioSessionOptions) (string, string, error) {
	args := m.Called(offerSDP, opts)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *MockSessionManager) DeleteAudioSession(sessionID string) error {
	args := m.Called(sessionID)
	return args.Error(0)
}

func (m *MockSessionManager) CreateVideoSession(offerSDP string, opts rtc.VideoSessionOptions) (string, string, error) {
	args := m.Called(offerSDP, opts)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *MockSessionManager) DeleteVideoSession(sessionID string) error {
	args := m.Called(sessionID)
	return args.Error(0)
}

type MockRobotGateway struct {
	mock.Mock
}

func (m *MockRobotGateway) StartRTSPPusher() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockRobotGateway) StopRTSPPusher() error {
	args := m.Called()
	return args.Error(0)
}

// setup function for unittest
func setupTestRouter(h *handlers.Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.POST("/control/whip", h.HandleControlWHIP)
	router.DELETE("/control/whip/:id", h.DeleteControlWHIP)
	router.POST("/audio/whip", h.HandleAudioWHIP)
	router.DELETE("/audio/whip/:id", h.DeleteAudioWHIP)
	router.POST("/video/whep", h.HandleVideoWHEP)
	router.DELETE("/video/whep/:id", h.DeleteVideoWHEP)

	return router
}

func TestNewHandler(t *testing.T) {
	// Arrange
	appConfig := &config.AppConfig{}
	gConfig := &config.GStreamerConfig{}

	// Act
	handler := handlers.NewHandler(appConfig, gConfig, &MockSessionManager{}, &MockRobotGateway{})

	// Assert
	require.NotNil(t, handler)
	assert.Equal(t, appConfig, handler.App)
	assert.Equal(t, gConfig, handler.GStreamerCfg)
	assert.NotNil(t, handler.SessionManager)
}

// =============================
// Control Handler Unittest
// =============================

func TestHandleControlWHIP_Success(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	// mock session manager response
	offerSDP := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\n"
	expectedSessionID := "session-123"
	expectedAnswerSDP := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\na=answer\r\n"

	mockSM.On("CreateSession", mock.Anything, offerSDP, mock.Anything).
		Return(expectedSessionID, expectedAnswerSDP, nil)

	// Act
	req := httptest.NewRequest(http.MethodPost, "/control/whip", strings.NewReader(offerSDP))
	req.Header.Set("Content-Type", "application/sdp")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "application/sdp", w.Header().Get("Content-Type"))
	assert.Equal(t, "/control/whip/"+expectedSessionID, w.Header().Get("Location"))
	assert.Equal(t, expectedAnswerSDP, w.Body.String())
	mockSM.AssertExpectations(t)
}

func TestHandleControlWHIP_InvalidContentType(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	// Act
	req := httptest.NewRequest(http.MethodPost, "/control/whip", strings.NewReader("test"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid Content-Type")
}

func TestHandleControlWHIP_CreateSessionError(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	offerSDP := "v=0\r\n"
	mockSM.On("CreateSession", mock.Anything, offerSDP, mock.Anything).
		Return("", "", errors.New("connection failed"))

	// Act
	req := httptest.NewRequest(http.MethodPost, "/control/whip", strings.NewReader(offerSDP))
	req.Header.Set("Content-Type", "application/sdp")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to create WebRTC session")
	mockSM.AssertExpectations(t)
}

func TestDeleteControlWHIP_Success(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	sessionID := "session-123"
	mockSM.On("DeleteSession", sessionID).Return(nil)

	// Act
	req := httptest.NewRequest(http.MethodDelete, "/control/whip/"+sessionID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	mockSM.AssertExpectations(t)
}

func TestDeleteControlWHIP_NotFound(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	sessionID := "session-123"
	mockSM.On("DeleteSession", sessionID).Return(errors.New("session not found"))

	// Act
	req := httptest.NewRequest(http.MethodDelete, "/control/whip/"+sessionID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Session not found")
	mockSM.AssertExpectations(t)
}

// =============================
// Audio Handler Unittest
// =============================

func TestHandleAudioWHIP_Success(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	offerSDP := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\n"
	expectedSessionID := "audio-session-123"
	expectedAnswerSDP := "v=0\r\na=audio-answer\r\n"

	mockSM.On("CreateAudioSession", offerSDP, mock.Anything).
		Return(expectedSessionID, expectedAnswerSDP, nil)

	// Act
	req := httptest.NewRequest(http.MethodPost, "/audio/whip", strings.NewReader(offerSDP))
	req.Header.Set("Content-Type", "application/sdp")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "application/sdp", w.Header().Get("Content-Type"))
	assert.Equal(t, "/audio/whip/"+expectedSessionID, w.Header().Get("Location"))
	assert.Equal(t, expectedAnswerSDP, w.Body.String())
	mockSM.AssertExpectations(t)
}

func TestHandleAudioWHIP_InvalidContentType(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	// Act
	req := httptest.NewRequest(http.MethodPost, "/audio/whip", strings.NewReader("test"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid Content-Type")
}

func TestHandleAudioWHIP_CreateSessionError(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	offerSDP := "v=0\r\n"
	mockSM.On("CreateAudioSession", offerSDP, mock.Anything).
		Return("", "", errors.New("audio error"))

	// Act
	req := httptest.NewRequest(http.MethodPost, "/audio/whip", strings.NewReader(offerSDP))
	req.Header.Set("Content-Type", "application/sdp")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to create audio session")
	mockSM.AssertExpectations(t)
}

func TestDeleteAudioWHIP_Success(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	sessionID := "audio-session-123"
	mockSM.On("DeleteAudioSession", sessionID).Return(nil)

	// Act
	req := httptest.NewRequest(http.MethodDelete, "/audio/whip/"+sessionID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	mockSM.AssertExpectations(t)
}

func TestDeleteAudioWHIP_NotFound(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	sessionID := "non-existent"
	mockSM.On("DeleteAudioSession", sessionID).Return(errors.New("not found"))

	// Act
	req := httptest.NewRequest(http.MethodDelete, "/audio/whip/"+sessionID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusNotFound, w.Code)
	mockSM.AssertExpectations(t)
}

// =============================
// Video Handler Unittest
// =============================
func TestHandleVideoWHEP_Success(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	offerSDP := "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\n"
	expectedSessionID := "video-session-123"
	expectedAnswerSDP := "v=0\r\na=video-answer\r\n"

	mockSM.On("CreateVideoSession", offerSDP, mock.Anything).
		Return(expectedSessionID, expectedAnswerSDP, nil)
	mockRobotGateway.On("StartRTSPPusher").Return(nil)

	// Act
	req := httptest.NewRequest(http.MethodPost, "/video/whep", strings.NewReader(offerSDP))
	req.Header.Set("Content-Type", "application/sdp")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "application/sdp", w.Header().Get("Content-Type"))
	assert.Equal(t, "/video/whep/"+expectedSessionID, w.Header().Get("Location"))
	assert.Equal(t, expectedAnswerSDP, w.Body.String())
	mockSM.AssertExpectations(t)
	mockRobotGateway.AssertExpectations(t)
}

func TestHandleVideoWHEP_InvalidContentType(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	// Act
	req := httptest.NewRequest(http.MethodPost, "/video/whep", strings.NewReader("test"))
	req.Header.Set("Content-Type", "video/mp4")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
}

func TestHandleVideoWHEP_CreateSessionError(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	offerSDP := "v=0\r\n"
	mockSM.On("CreateVideoSession", offerSDP, mock.Anything).
		Return("", "", errors.New("video error"))

	// Act
	req := httptest.NewRequest(http.MethodPost, "/video/whep", strings.NewReader(offerSDP))
	req.Header.Set("Content-Type", "application/sdp")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSM.AssertExpectations(t)
}

func TestDeleteVideoWHEP_Success(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	sessionID := "video-session-123"
	mockRobotGateway.On("StopRTSPPusher").Return(nil)
	mockSM.On("DeleteVideoSession", sessionID).Return(nil)

	// Act
	req := httptest.NewRequest(http.MethodDelete, "/video/whep/"+sessionID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	mockSM.AssertExpectations(t)
	mockRobotGateway.AssertExpectations(t)
}

func TestDeleteVideoWHEP_NotFound(t *testing.T) {
	// Arrange
	mockSM := new(MockSessionManager)
	mockRobotGateway := new(MockRobotGateway)
	handler := handlers.NewHandler(&config.AppConfig{}, &config.GStreamerConfig{}, mockSM, mockRobotGateway)
	router := setupTestRouter(handler)

	sessionID := "non-existent"
	mockRobotGateway.On("StopRTSPPusher").Return(nil)
	mockSM.On("DeleteVideoSession", sessionID).Return(errors.New("not found"))

	// Act
	req := httptest.NewRequest(http.MethodDelete, "/video/whep/"+sessionID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusNotFound, w.Code)
	mockSM.AssertExpectations(t)
}
