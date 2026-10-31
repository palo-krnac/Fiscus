//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

const itemCols = "id, company_id, code, name, unit, price, price_mode, vat_rate, note, archived, ean, category, brand, supplier, supplier_code, purchase_price, weight, description, params_json"

func scanItem(r interface{ Scan(...any) error }) (catalog.Item, error) {
	var it catalog.Item
	var price int64
	var mode string
	var rate int
	var purchase, weight int64
	var params string
	err := r.Scan(&it.ID, &it.CompanyID, &it.Code, &it.Name, &it.Unit, &price, &mode, &rate, &it.Note, &it.Archived,
		&it.EAN, &it.Category, &it.Brand, &it.Supplier, &it.SupplierCode, &purchase, &weight, &it.Description, &params)
	it.Price, it.PriceMode, it.Rate = money.FromRaw(price), vat.PriceMode(mode), vat.Rate(rate)
	it.PurchasePrice, it.Weight = money.FromRaw(purchase), money.FromRaw(weight)
	if err == nil && params != "" {
		err = json.Unmarshal([]byte(params), &it.Params)
	}
	return it, err
}

func (s *Store) ListItems(ctx context.Context, companyID int64) ([]catalog.Item, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+itemCols+" FROM catalog_items WHERE company_id = ?", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// poradie podľa názvu bez diakritiky (SQLite NOCASE nepozná slovenčinu)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := store.SearchKey(out[i].Name), store.SearchKey(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) GetItem(ctx context.Context, companyID, id int64) (catalog.Item, error) {
	it, err := scanItem(s.db.QueryRowContext(ctx, "SELECT "+itemCols+" FROM catalog_items WHERE id = ? AND company_id = ?", id, companyID))
	if errors.Is(err, sql.ErrNoRows) {
		return it, store.ErrNotFound
	}
	return it, err
}

func (s *Store) SaveItem(ctx context.Context, companyID int64, it *catalog.Item) error {
	params := ""
	if len(it.Params) > 0 {
		b, err := json.Marshal(it.Params)
		if err != nil {
			return err
		}
		params = string(b)
	}
	vals := []any{it.Code, it.Name, it.Unit, it.Price.Raw(), string(it.PriceMode), int(it.Rate), it.Note, it.Archived,
		it.EAN, it.Category, it.Brand, it.Supplier, it.SupplierCode, it.PurchasePrice.Raw(), it.Weight.Raw(), it.Description, params}
	if it.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO catalog_items (company_id, code, name, unit, price, price_mode, vat_rate, note, archived,
			ean, category, brand, supplier, supplier_code, purchase_price, weight, description, params_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, append([]any{companyID}, vals...)...)
		if isUnique(err) {
			return store.ErrDuplicate
		}
		if err != nil {
			return err
		}
		if it.ID, err = res.LastInsertId(); err != nil {
			return err
		}
	} else {
		res, err := s.db.ExecContext(ctx, `UPDATE catalog_items SET code = ?, name = ?, unit = ?, price = ?, price_mode = ?, vat_rate = ?,
			note = ?, archived = ?, ean = ?, category = ?, brand = ?, supplier = ?, supplier_code = ?, purchase_price = ?, weight = ?,
			description = ?, params_json = ? WHERE id = ? AND company_id = ?`, append(vals, it.ID, companyID)...)
		if isUnique(err) {
			return store.ErrDuplicate
		}
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return store.ErrNotFound
		}
	}
	it.CompanyID = companyID
	return nil
}

func (s *Store) DeleteItem(ctx context.Context, companyID, id int64) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM item_images WHERE item_id = ? AND company_id = ?", id, companyID); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM catalog_items WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

const imageCols = "id, company_id, item_id, name, content_type, size, position, created"

func scanImage(r interface{ Scan(...any) error }, data *[]byte) (store.ItemImage, error) {
	var im store.ItemImage
	var created string
	dest := []any{&im.ID, &im.CompanyID, &im.ItemID, &im.Name, &im.ContentType, &im.Size, &im.Position, &created}
	if data != nil {
		dest = append(dest, data)
	}
	err := r.Scan(dest...)
	im.Created, _ = time.Parse(time.RFC3339, created)
	return im, err
}

func (s *Store) ListItemImages(ctx context.Context, companyID, itemID int64) ([]store.ItemImage, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+imageCols+" FROM item_images WHERE company_id = ? AND item_id = ? ORDER BY position, id", companyID, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []store.ItemImage{}
	for rows.Next() {
		im, err := scanImage(rows, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, im)
	}
	return out, rows.Err()
}

func (s *Store) GetItemImage(ctx context.Context, companyID, id int64) (store.ItemImage, error) {
	var data []byte
	im, err := scanImage(s.db.QueryRowContext(ctx, "SELECT "+imageCols+", data FROM item_images WHERE id = ? AND company_id = ?", id, companyID), &data)
	if errors.Is(err, sql.ErrNoRows) {
		return im, store.ErrNotFound
	}
	im.Data = data
	return im, err
}

func (s *Store) AddItemImage(ctx context.Context, img *store.ItemImage) error {
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM catalog_items WHERE id = ? AND company_id = ?", img.ItemID, img.CompanyID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	var pos sql.NullInt64
	if err := s.db.QueryRowContext(ctx, "SELECT MAX(position) FROM item_images WHERE item_id = ?", img.ItemID).Scan(&pos); err != nil {
		return err
	}
	img.Position, img.Size = 0, len(img.Data)
	if pos.Valid {
		img.Position = int(pos.Int64) + 1
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO item_images (company_id, item_id, name, content_type, size, position, data, created)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, img.CompanyID, img.ItemID, img.Name, img.ContentType, img.Size, img.Position, img.Data,
		img.Created.UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	img.ID, err = res.LastInsertId()
	return err
}

func (s *Store) SetItemImageOrder(ctx context.Context, companyID, itemID int64, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		res, err := tx.ExecContext(ctx, "UPDATE item_images SET position = ? WHERE id = ? AND item_id = ? AND company_id = ?", i, id, itemID, companyID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return store.ErrNotFound
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteItemImage(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM item_images WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
