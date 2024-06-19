package tdlib

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/zelenin/go-tdlib/client"
)

type TDlib struct {
	client *client.Client
}

func NewTDlib(apiID int32, apiHash string) (TDlib, error) {
	// client authorizer
	authorizer := client.ClientAuthorizer()
	go client.CliInteractor(authorizer)

	authorizer.TdlibParameters <- &client.SetTdlibParametersRequest{
		UseTestDc:           false,
		DatabaseDirectory:   filepath.Join(".tdlib", "database"),
		FilesDirectory:      filepath.Join(".tdlib", "files"),
		UseFileDatabase:     true,
		UseChatInfoDatabase: true,
		UseMessageDatabase:  true,
		UseSecretChats:      false,
		ApiId:               apiID,
		ApiHash:             apiHash,
		SystemLanguageCode:  "en",
		DeviceModel:         "Server",
		SystemVersion:       "1.0.0",
		ApplicationVersion:  "1.0.0",
	}

	_, err := client.SetLogVerbosityLevel(&client.SetLogVerbosityLevelRequest{
		NewVerbosityLevel: 1,
	})
	if err != nil {
		log.Fatalf("Ошибка SetLogVerbosityLevel: %s", err)
	}

	tdlibClient, err := client.NewClient(authorizer)
	if err != nil {
		return TDlib{}, fmt.Errorf(fmt.Sprintf("ошибка создания NewClient: %s", err.Error()))
	}

	return TDlib{client: tdlibClient}, nil
}

func (t *TDlib) TDlibStop() {
	t.client.Stop()
}

// CreateNewGroup создает новую группу в telegram
func (t *TDlib) CreateNewGroup(title string) (groupID int64, err error) {
	req := &client.CreateNewSupergroupChatRequest{Title: title}

	chat, err := t.client.CreateNewSupergroupChat(req)
	if err != nil {
		return 0, fmt.Errorf("ошибка создания группы: %s", err)
	}

	return chat.Id, nil
}

// AddUserToGroup добавляет пользователя в группу
func (t *TDlib) AddUserToGroup(groupID, userID int64) error {
	req := &client.AddChatMemberRequest{ChatId: groupID, UserId: userID}
	_, err := t.client.AddChatMember(req)
	if err != nil {
		return fmt.Errorf("ошибка добавления участника %d в группу: %s", userID, err)
	}

	return nil
}

// GetGroup ищет группу (чат группы) в telegram
func (t *TDlib) GetGroup(groupID int64) (*client.Chat, error) {
	req := &client.GetChatRequest{ChatId: groupID}

	chat, err := t.client.GetChat(req)
	if err != nil {
		return nil, err
	}

	return chat, nil
}
