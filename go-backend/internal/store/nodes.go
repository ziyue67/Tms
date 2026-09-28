package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/database"
)

type Node struct {
	ID          int64          `json:"id"`
	CreatedTime int64          `json:"createdTime"`
	UpdatedTime sql.NullInt64  `json:"updatedTime"`
	Status      int            `json:"status"`
	Name        string         `json:"name"`
	Secret      sql.NullString `json:"secret"`
	IP          string         `json:"ip"`
	ServerIP    string         `json:"serverIp"`
	Domain      sql.NullString `json:"domain"`
	Version     sql.NullString `json:"version"`
	PortStart   int            `json:"portSta"`
	PortEnd     int            `json:"portEnd"`
	HTTP        int            `json:"http"`
	TLS         int            `json:"tls"`
	SOCKS       int            `json:"socks"`
}

func (n Node) MarshalJSON() ([]byte, error) {
	type alias struct {
		ID          int64   `json:"id"`
		CreatedTime int64   `json:"createdTime"`
		UpdatedTime *int64  `json:"updatedTime"`
		Status      int     `json:"status"`
		Name        string  `json:"name"`
		Secret      *string `json:"secret"`
		IP          string  `json:"ip"`
		ServerIP    string  `json:"serverIp"`
		Domain      *string `json:"domain"`
		Version     *string `json:"version"`
		PortStart   int     `json:"portSta"`
		PortEnd     int     `json:"portEnd"`
		HTTP        int     `json:"http"`
		TLS         int     `json:"tls"`
		SOCKS       int     `json:"socks"`
	}
	return json.Marshal(alias{ID: n.ID, CreatedTime: n.CreatedTime, UpdatedTime: nullInt(n.UpdatedTime), Status: n.Status,
		Name: n.Name, Secret: nullString(n.Secret), IP: n.IP, ServerIP: n.ServerIP, Domain: nullString(n.Domain),
		Version: nullString(n.Version), PortStart: n.PortStart, PortEnd: n.PortEnd, HTTP: n.HTTP, TLS: n.TLS, SOCKS: n.SOCKS})
}

type NodeInput struct {
	Name      string
	IP        string
	ServerIP  string
	Domain    string
	PortStart int
	PortEnd   int
	HTTP      *int
	TLS       *int
	SOCKS     *int
}

var protocolForwardName = regexp.MustCompile(`^inbound-(\d+)-user-\d+$`)

func (s *Store) Nodes(ctx context.Context) ([]Node, error) {
	query := "SELECT " + s.columns("id", "created_time", "updated_time", "status", "name", "secret", "ip", "server_ip", "domain", "version", "port_sta", "port_end", "http", "tls", "socks") + " FROM " + s.quote("node") + " ORDER BY " + s.quote("id")
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Node, 0)
	for rows.Next() {
		var node Node
		if err := scanNode(rows, &node); err != nil {
			return nil, err
		}
		result = append(result, node)
	}
	return result, rows.Err()
}

func (s *Store) NodeByID(ctx context.Context, id int64) (Node, error) {
	query := "SELECT " + s.columns("id", "created_time", "updated_time", "status", "name", "secret", "ip", "server_ip", "domain", "version", "port_sta", "port_end", "http", "tls", "socks") + " FROM " + s.quote("node") + " WHERE " + s.quote("id") + " = ?"
	var node Node
	err := scanNode(s.db.QueryRowContext(ctx, s.bind(query), id), &node)
	return node, err
}

type scanner interface{ Scan(...any) error }

func scanNode(row scanner, node *Node) error {
	return row.Scan(&node.ID, &node.CreatedTime, &node.UpdatedTime, &node.Status, &node.Name, &node.Secret, &node.IP,
		&node.ServerIP, &node.Domain, &node.Version, &node.PortStart, &node.PortEnd, &node.HTTP, &node.TLS, &node.SOCKS)
}

func (s *Store) CreateNode(ctx context.Context, input NodeInput) (Node, error) {
	secret, err := randomSecret()
	if err != nil {
		return Node{}, err
	}
	now := time.Now().UnixMilli()
	query := "INSERT INTO " + s.quote("node") + " (" + s.columns("name", "secret", "ip", "server_ip", "domain", "port_sta", "port_end", "created_time", "updated_time", "status") + ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0)"
	args := []any{input.Name, secret, input.IP, input.ServerIP, nullableString(input.Domain), input.PortStart, input.PortEnd, now, now}
	var id int64
	if s.dialect == database.PostgreSQL {
		if err := s.db.QueryRowContext(ctx, s.bind(query+" RETURNING "+s.quote("id")), args...).Scan(&id); err != nil {
			return Node{}, err
		}
	} else {
		result, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			return Node{}, err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return Node{}, err
		}
	}
	return Node{ID: id, CreatedTime: now, UpdatedTime: sql.NullInt64{Int64: now, Valid: true}, Name: input.Name,
		Secret: sql.NullString{String: secret, Valid: true}, IP: input.IP, ServerIP: input.ServerIP,
		Domain: sql.NullString{String: input.Domain, Valid: input.Domain != ""}, PortStart: input.PortStart, PortEnd: input.PortEnd}, nil
}

func (s *Store) UpdateNode(ctx context.Context, id int64, input NodeInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	assignments := []string{s.quote("name") + " = ?", s.quote("ip") + " = ?", s.quote("server_ip") + " = ?",
		s.quote("domain") + " = ?", s.quote("port_sta") + " = ?", s.quote("port_end") + " = ?", s.quote("updated_time") + " = ?"}
	args := []any{input.Name, input.IP, input.ServerIP, nullableString(input.Domain), input.PortStart, input.PortEnd, time.Now().UnixMilli()}
	for column, value := range map[string]*int{"http": input.HTTP, "tls": input.TLS, "socks": input.SOCKS} {
		if value != nil {
			assignments = append(assignments, s.quote(column)+" = ?")
			args = append(args, *value)
		}
	}
	args = append(args, id)
	query := "UPDATE " + s.quote("node") + " SET " + strings.Join(assignments, ", ") + " WHERE " + s.quote("id") + " = ?"
	result, err := tx.ExecContext(ctx, s.bind(query), args...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return fmt.Errorf("节点更新失败")
	}
	if _, err := tx.ExecContext(ctx, s.bind("UPDATE "+s.quote("tunnel")+" SET "+s.quote("in_ip")+" = ? WHERE "+s.quote("in_node_id")+" = ?"), input.IP, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.bind("UPDATE "+s.quote("tunnel")+" SET "+s.quote("out_ip")+" = ? WHERE "+s.quote("out_node_id")+" = ?"), input.ServerIP, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RenameNode(ctx context.Context, id int64, name string) (bool, error) {
	query := "UPDATE " + s.quote("node") + " SET " + s.quote("name") + " = ?, " + s.quote("updated_time") + " = ? WHERE " + s.quote("id") + " = ?"
	result, err := s.db.ExecContext(ctx, s.bind(query), name, time.Now().UnixMilli(), id)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (s *Store) DeleteNode(ctx context.Context, nodeID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	count, err := countQuery(ctx, tx, s.bind("SELECT COUNT(*) FROM "+s.quote("inbound")+" WHERE "+s.quote("node_id")+" = ?"), nodeID)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("该节点还有 %d 个协议，请先删除相关协议", count)
	}

	tunnels, err := s.protocolTunnels(ctx, tx, nodeID)
	if err != nil {
		return err
	}
	for _, tunnel := range tunnels {
		if err := s.pruneDanglingForwards(ctx, tx, tunnel.id); err != nil {
			return err
		}
	}
	orphans := make(map[int64]bool)
	for _, tunnel := range tunnels {
		forwards, err := countQuery(ctx, tx, s.bind("SELECT COUNT(*) FROM "+s.quote("forward")+" WHERE "+s.quote("tunnel_id")+" = ?"), tunnel.id)
		if err != nil {
			return err
		}
		permissions, err := countQuery(ctx, tx, s.bind("SELECT COUNT(*) FROM "+s.quote("user_tunnel")+" WHERE "+s.quote("tunnel_id")+" = ?"), tunnel.id)
		if err != nil {
			return err
		}
		orphans[tunnel.id] = forwards == 0 && permissions == 0
	}
	allTunnels, err := s.nodeTunnels(ctx, tx, nodeID)
	if err != nil {
		return err
	}
	inUse, outUse := 0, 0
	for _, tunnel := range allTunnels {
		if orphans[tunnel.id] {
			continue
		}
		if tunnel.inNodeID == nodeID {
			inUse++
		}
		if tunnel.outNodeID == nodeID {
			outUse++
		}
	}
	if inUse > 0 {
		return fmt.Errorf("该节点还有 %d 个隧道作为入口节点在使用，请先删除相关隧道", inUse)
	}
	if outUse > 0 {
		return fmt.Errorf("该节点还有 %d 个隧道作为出口节点在使用，请先删除相关隧道", outUse)
	}
	for _, table := range []string{"inbound_line", "inbound_auto_provision"} {
		if _, err := tx.ExecContext(ctx, s.bind("DELETE FROM "+s.quote(table)+" WHERE "+s.quote("node_id")+" = ?"), nodeID); err != nil {
			return err
		}
	}
	for tunnelID, orphan := range orphans {
		if !orphan {
			continue
		}
		if _, err := tx.ExecContext(ctx, s.bind("DELETE FROM "+s.quote("tunnel")+" WHERE "+s.quote("id")+" = ?"), tunnelID); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, s.bind("DELETE FROM "+s.quote("node")+" WHERE "+s.quote("id")+" = ?"), nodeID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return errors.New("节点删除失败")
	}
	return tx.Commit()
}

type tunnelRef struct{ id, inNodeID, outNodeID int64 }

func (s *Store) protocolTunnels(ctx context.Context, tx *sql.Tx, nodeID int64) ([]tunnelRef, error) {
	query := "SELECT " + s.columns("id", "in_node_id", "out_node_id") + " FROM " + s.quote("tunnel") + " WHERE " +
		s.quote("in_node_id") + " = ? AND " + s.quote("out_node_id") + " = ? AND " + s.quote("type") + " = 1 AND " + s.quote("name") + " LIKE 'inbound-tunnel-node%'"
	return scanTunnelRefs(ctx, tx, s.bind(query), nodeID, nodeID)
}

func (s *Store) nodeTunnels(ctx context.Context, tx *sql.Tx, nodeID int64) ([]tunnelRef, error) {
	query := "SELECT " + s.columns("id", "in_node_id", "out_node_id") + " FROM " + s.quote("tunnel") + " WHERE " + s.quote("in_node_id") + " = ? OR " + s.quote("out_node_id") + " = ?"
	return scanTunnelRefs(ctx, tx, s.bind(query), nodeID, nodeID)
}

func scanTunnelRefs(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]tunnelRef, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []tunnelRef
	for rows.Next() {
		var tunnel tunnelRef
		if err := rows.Scan(&tunnel.id, &tunnel.inNodeID, &tunnel.outNodeID); err != nil {
			return nil, err
		}
		result = append(result, tunnel)
	}
	return result, rows.Err()
}

func (s *Store) pruneDanglingForwards(ctx context.Context, tx *sql.Tx, tunnelID int64) error {
	query := "SELECT " + s.columns("id", "name") + " FROM " + s.quote("forward") + " WHERE " + s.quote("tunnel_id") + " = ?"
	rows, err := tx.QueryContext(ctx, s.bind(query), tunnelID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type forwardRef struct {
		id   int64
		name string
	}
	var forwards []forwardRef
	for rows.Next() {
		var forward forwardRef
		if err := rows.Scan(&forward.id, &forward.name); err != nil {
			return err
		}
		forwards = append(forwards, forward)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, forward := range forwards {
		match := protocolForwardName.FindStringSubmatch(forward.name)
		if len(match) != 2 {
			continue
		}
		inboundID, _ := strconv.ParseInt(match[1], 10, 64)
		exists, err := countQuery(ctx, tx, s.bind("SELECT COUNT(*) FROM "+s.quote("inbound")+" WHERE "+s.quote("id")+" = ?"), inboundID)
		if err != nil {
			return err
		}
		if exists == 0 {
			if _, err := tx.ExecContext(ctx, s.bind("DELETE FROM "+s.quote("forward")+" WHERE "+s.quote("id")+" = ?"), forward.id); err != nil {
				return err
			}
		}
	}
	return nil
}

func countQuery(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, query string, args ...any) (int, error) {
	var count int
	err := queryer.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

func randomSecret() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func nullInt(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func nullString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
