package main

import (
	"database/sql"
	"fmt"
	"strings"
)

type listsRequest struct {
	Action      string
	ListName    string
	ItemID      int
	ItemContent string
	Status      string
}

type listInfo struct {
	ID           int
	Name         string
	TotalItems   int
	PendingItems int
}

type listItem struct {
	ID        int
	Content   string
	Status    string
	CreatedAt string
}

type listsResult struct {
	Request      listsRequest
	Lists        []listInfo
	Items        []listItem
	CreatedItem  *listItem
	UpdatedItem  *listItem
	ExecutionErr string
	UserDenied   bool
}

func parseListsRequest(args map[string]any) (listsRequest, error) {
	req := listsRequest{}

	action, err := requiredStringArg(args, "action")
	if err != nil {
		return req, err
	}
	req.Action = action

	// Optional parameters
	if v, ok := args["list_name"]; ok {
		if s, ok := v.(string); ok {
			req.ListName = strings.TrimSpace(s)
		}
	}

	if v, ok := args["item_id"]; ok {
		itemID, err := parseInt(v)
		if err != nil {
			return req, fmt.Errorf("item_id must be an integer: %w", err)
		}
		req.ItemID = itemID
	}

	if v, ok := args["item_content"]; ok {
		if s, ok := v.(string); ok {
			req.ItemContent = strings.TrimSpace(s)
		}
	}

	if v, ok := args["status"]; ok {
		if s, ok := v.(string); ok {
			req.Status = strings.TrimSpace(s)
		}
	}

	// Validate status if provided
	if req.Status != "" && req.Status != "pending" && req.Status != "done" {
		return req, fmt.Errorf("invalid status '%s': must be 'pending' or 'done'", req.Status)
	}

	return req, nil
}

func executeLists(db *sql.DB, req listsRequest, autoApprove bool) listsResult {
	res := listsResult{Request: req}

	switch req.Action {
	case "get_lists":
		lists, err := getLists(db)
		if err != nil {
			res.ExecutionErr = err.Error()
			return res
		}
		res.Lists = lists

	case "create_list":
		if req.ListName == "" {
			res.ExecutionErr = "list_name is required for create_list action"
			return res
		}
		if err := createList(db, req.ListName); err != nil {
			res.ExecutionErr = err.Error()
			return res
		}

	case "delete_list":
		if req.ListName == "" {
			res.ExecutionErr = "list_name is required for delete_list action"
			return res
		}

		// Check if list exists and get item count
		listID, itemCount, err := getListInfo(db, req.ListName)
		if err != nil {
			res.ExecutionErr = err.Error()
			return res
		}

		// Ask for approval unless --yolo is active
		if !autoApprove {
			printDeleteListPrompt(req.ListName, itemCount)
			if !askForDeleteListApproval() {
				res.UserDenied = true
				res.ExecutionErr = "list deletion denied by user"
				return res
			}
		}

		if err := deleteList(db, listID); err != nil {
			res.ExecutionErr = err.Error()
			return res
		}

	case "get_items":
		if req.ListName == "" {
			res.ExecutionErr = "list_name is required for get_items action"
			return res
		}
		items, err := getItems(db, req.ListName)
		if err != nil {
			res.ExecutionErr = err.Error()
			return res
		}
		res.Items = items

	case "add_item":
		if req.ListName == "" {
			res.ExecutionErr = "list_name is required for add_item action"
			return res
		}
		if req.ItemContent == "" {
			res.ExecutionErr = "item_content is required for add_item action"
			return res
		}

		// Default status to pending if not provided
		status := req.Status
		if status == "" {
			status = "pending"
		}

		item, err := addItem(db, req.ListName, req.ItemContent, status)
		if err != nil {
			res.ExecutionErr = err.Error()
			return res
		}
		res.CreatedItem = &item

	case "update_item":
		if req.ItemID == 0 {
			res.ExecutionErr = "item_id is required for update_item action"
			return res
		}
		if req.ItemContent == "" && req.Status == "" {
			res.ExecutionErr = "either item_content or status must be provided for update_item action"
			return res
		}

		item, err := updateItem(db, req.ItemID, req.ItemContent, req.Status)
		if err != nil {
			res.ExecutionErr = err.Error()
			return res
		}
		res.UpdatedItem = &item

	case "delete_item":
		if req.ItemID == 0 {
			res.ExecutionErr = "item_id is required for delete_item action"
			return res
		}
		if err := deleteItem(db, req.ItemID); err != nil {
			res.ExecutionErr = err.Error()
			return res
		}

	default:
		res.ExecutionErr = fmt.Sprintf("unknown action '%s': must be one of create_list, delete_list, get_lists, add_item, update_item, delete_item, get_items", req.Action)
	}

	return res
}

func (res listsResult) toToolResponse() map[string]any {
	if res.ExecutionErr != "" {
		return map[string]any{
			"error": map[string]any{
				"message": res.ExecutionErr,
			},
		}
	}

	if res.UserDenied {
		return map[string]any{
			"error": map[string]any{
				"message": "operation denied by user",
			},
		}
	}

	response := map[string]any{"ok": true}

	switch res.Request.Action {
	case "get_lists":
		lists := make([]map[string]any, 0, len(res.Lists))
		for _, list := range res.Lists {
			lists = append(lists, map[string]any{
				"id":            list.ID,
				"name":          list.Name,
				"total_items":   list.TotalItems,
				"pending_items": list.PendingItems,
			})
		}
		response["lists"] = lists
		response["count"] = len(lists)

	case "create_list":
		response["list_name"] = res.Request.ListName

	case "delete_list":
		response["list_name"] = res.Request.ListName

	case "get_items":
		items := make([]map[string]any, 0, len(res.Items))
		for _, item := range res.Items {
			items = append(items, map[string]any{
				"id":         item.ID,
				"content":    item.Content,
				"status":     item.Status,
				"created_at": item.CreatedAt,
			})
		}
		response["items"] = items
		response["count"] = len(items)

	case "add_item":
		if res.CreatedItem != nil {
			response["item"] = map[string]any{
				"id":         res.CreatedItem.ID,
				"content":    res.CreatedItem.Content,
				"status":     res.CreatedItem.Status,
				"created_at": res.CreatedItem.CreatedAt,
			}
		}

	case "update_item":
		if res.UpdatedItem != nil {
			response["item"] = map[string]any{
				"id":         res.UpdatedItem.ID,
				"content":    res.UpdatedItem.Content,
				"status":     res.UpdatedItem.Status,
				"created_at": res.UpdatedItem.CreatedAt,
			}
		}

	case "delete_item":
		response["item_id"] = res.Request.ItemID
	}

	return response
}

// Database operations

func getLists(db *sql.DB) ([]listInfo, error) {
	query := `
		SELECT 
			l.id,
			l.name,
			COUNT(li.id) as total_items,
			SUM(CASE WHEN li.status = 'pending' THEN 1 ELSE 0 END) as pending_items
		FROM lists l
		LEFT JOIN list_items li ON l.id = li.list_id
		GROUP BY l.id, l.name
		ORDER BY l.name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query lists: %w", err)
	}
	defer rows.Close()

	var lists []listInfo
	for rows.Next() {
		var list listInfo
		var pendingItems sql.NullInt64
		if err := rows.Scan(&list.ID, &list.Name, &list.TotalItems, &pendingItems); err != nil {
			return nil, fmt.Errorf("failed to scan list row: %w", err)
		}
		if pendingItems.Valid {
			list.PendingItems = int(pendingItems.Int64)
		}
		lists = append(lists, list)
	}

	return lists, nil
}

func createList(db *sql.DB, name string) error {
	_, err := db.Exec("INSERT INTO lists (name) VALUES (?)", name)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("list '%s' already exists", name)
		}
		return fmt.Errorf("failed to create list: %w", err)
	}
	return nil
}

func getListInfo(db *sql.DB, name string) (int, int, error) {
	var listID int
	var itemCount int

	query := `
		SELECT l.id, COUNT(li.id)
		FROM lists l
		LEFT JOIN list_items li ON l.id = li.list_id
		WHERE l.name = ?
		GROUP BY l.id
	`

	err := db.QueryRow(query, name).Scan(&listID, &itemCount)
	if err == sql.ErrNoRows {
		return 0, 0, fmt.Errorf("list '%s' not found", name)
	}
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get list info: %w", err)
	}

	return listID, itemCount, nil
}

func deleteList(db *sql.DB, listID int) error {
	_, err := db.Exec("DELETE FROM lists WHERE id = ?", listID)
	if err != nil {
		return fmt.Errorf("failed to delete list: %w", err)
	}
	return nil
}

func getItems(db *sql.DB, listName string) ([]listItem, error) {
	// First check if list exists
	var listID int
	err := db.QueryRow("SELECT id FROM lists WHERE name = ?", listName).Scan(&listID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("list '%s' not found", listName)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query list: %w", err)
	}

	query := `
		SELECT id, content, status, created_at
		FROM list_items
		WHERE list_id = ?
		ORDER BY 
			CASE WHEN status = 'pending' THEN 0 ELSE 1 END,
			created_at
	`

	rows, err := db.Query(query, listID)
	if err != nil {
		return nil, fmt.Errorf("failed to query items: %w", err)
	}
	defer rows.Close()

	var items []listItem
	for rows.Next() {
		var item listItem
		if err := rows.Scan(&item.ID, &item.Content, &item.Status, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan item row: %w", err)
		}
		items = append(items, item)
	}

	return items, nil
}

func addItem(db *sql.DB, listName string, content string, status string) (listItem, error) {
	// First get list ID
	var listID int
	err := db.QueryRow("SELECT id FROM lists WHERE name = ?", listName).Scan(&listID)
	if err == sql.ErrNoRows {
		return listItem{}, fmt.Errorf("list '%s' not found", listName)
	}
	if err != nil {
		return listItem{}, fmt.Errorf("failed to query list: %w", err)
	}

	result, err := db.Exec(
		"INSERT INTO list_items (list_id, content, status) VALUES (?, ?, ?)",
		listID, content, status,
	)
	if err != nil {
		return listItem{}, fmt.Errorf("failed to add item: %w", err)
	}

	itemID, err := result.LastInsertId()
	if err != nil {
		return listItem{}, fmt.Errorf("failed to get item ID: %w", err)
	}

	// Fetch the created item
	var item listItem
	err = db.QueryRow(
		"SELECT id, content, status, created_at FROM list_items WHERE id = ?",
		itemID,
	).Scan(&item.ID, &item.Content, &item.Status, &item.CreatedAt)
	if err != nil {
		return listItem{}, fmt.Errorf("failed to fetch created item: %w", err)
	}

	return item, nil
}

func updateItem(db *sql.DB, itemID int, content string, status string) (listItem, error) {
	// Check if item exists
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM list_items WHERE id = ?)", itemID).Scan(&exists)
	if err != nil {
		return listItem{}, fmt.Errorf("failed to check item existence: %w", err)
	}
	if !exists {
		return listItem{}, fmt.Errorf("item with id %d not found", itemID)
	}

	// Build update query dynamically based on what's provided
	updates := []string{}
	args := []any{}

	if content != "" {
		updates = append(updates, "content = ?")
		args = append(args, content)
	}
	if status != "" {
		updates = append(updates, "status = ?")
		args = append(args, status)
	}

	// Always update updated_at
	updates = append(updates, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, itemID)

	query := fmt.Sprintf("UPDATE list_items SET %s WHERE id = ?", strings.Join(updates, ", "))
	_, err = db.Exec(query, args...)
	if err != nil {
		return listItem{}, fmt.Errorf("failed to update item: %w", err)
	}

	// Fetch the updated item
	var item listItem
	err = db.QueryRow(
		"SELECT id, content, status, created_at FROM list_items WHERE id = ?",
		itemID,
	).Scan(&item.ID, &item.Content, &item.Status, &item.CreatedAt)
	if err != nil {
		return listItem{}, fmt.Errorf("failed to fetch updated item: %w", err)
	}

	return item, nil
}

func deleteItem(db *sql.DB, itemID int) error {
	result, err := db.Exec("DELETE FROM list_items WHERE id = ?", itemID)
	if err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("item with id %d not found", itemID)
	}

	return nil
}

func initListsTables(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS lists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create lists table: %w", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS list_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
			content TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create list_items table: %w", err)
	}

	return nil
}

// UI functions for user prompts

func printDeleteListPrompt(listName string, itemCount int) {
	fmt.Printf("\n🗑️  Delete list '%s' and all its %d item(s)? [y/N]: ", listName, itemCount)
}

func askForDeleteListApproval() bool {
	return askYesNo()
}

func askYesNo() bool {
	var response string
	fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// Made with Bob
