package main

import "fmt"

type TelegramMediaRequest struct {
	FilePath string `json:"filepath"`
}

func parseDocumentSendRequest(args map[string]any) (TelegramMediaRequest, error) {
	fmt.Println("[DEBUG] parseDocumentSendRequest called")
	req := TelegramMediaRequest{}

	filepath, err := requiredStringArg(args, "filepath")
	if err != nil {
		fmt.Println("[ERROR] Failed to parse document filepath:", err)
		return req, err
	}
	req.FilePath = filepath
	fmt.Println("[DEBUG] Document filepath parsed:", filepath)

	return req, nil
}

func parseImageSendRequest(args map[string]any) (TelegramMediaRequest, error) {
	fmt.Println("[DEBUG] parseImageSendRequest called")
	req := TelegramMediaRequest{}

	filepath, err := requiredStringArg(args, "filepath")
	if err != nil {
		fmt.Println("[ERROR] Failed to parse image filepath:", err)
		return req, err
	}
	req.FilePath = filepath
	fmt.Println("[DEBUG] Image filepath parsed:", filepath)

	return req, nil
}
