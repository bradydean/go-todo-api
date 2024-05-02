package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	_ "github.com/joho/godotenv/autoload"

	"github.com/bradydean/go-todo-api/internal/pkg/jwtmiddleware"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	todo "github.com/bradydean/go-todo-api/internal/pkg/todo_api/todo/table"
	pg "github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ListRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type ListPartialRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

type ListResponse struct {
	ListID      int64  `json:"list_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type ItemRequest struct {
	Content    string `json:"content"`
	IsComplete bool   `json:"is_complete"`
}

type ItemPartialRequest struct {
	Content    *string `json:"content"`
	IsComplete *bool   `json:"is_complete"`
}

type ItemResponse struct {
	ItemID     int64  `json:"item_id"`
	Content    string `json:"content"`
	IsComplete bool   `json:"is_complete"`
}

type ListsRecord struct {
	ListID      int64  `db:"lists.list_id"`
	Title       string `db:"lists.title"`
	Description string `db:"lists.description"`
}

type ItemsRecord struct {
	ItemID     int64  `db:"items.item_id"`
	Content    string `db:"items.content"`
	IsComplete bool   `db:"items.is_complete"`
}

var NotFoundError = echo.NewHTTPError(
	http.StatusNotFound,
	map[string]string{"message": "Not found"},
)

var InternalServerError = echo.NewHTTPError(http.StatusInternalServerError)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		LogLevel: 4,
	}))
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:    true,
		LogURI:       true,
		LogLatency:   true,
		LogProtocol:  true,
		LogMethod:    true,
		LogUserAgent: true,
		LogRemoteIP:  true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			msg := fmt.Sprintf(
				"uri=%s status=%d latency=%s protocol=%s method=%s user_agent=%s remote_ip=%s",
				v.URI, v.Status, v.Latency, v.Protocol, v.Method, v.UserAgent, v.RemoteIP,
			)
			if v.Error == nil {
				logger.LogAttrs(context.Background(), slog.LevelInfo, msg,
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
					slog.Duration("latency", v.Latency),
					slog.String("protocol", v.Protocol),
					slog.String("method", v.Method),
					slog.String("user_agent", v.UserAgent),
					slog.String("remote_ip", v.RemoteIP),
				)
			} else {
				logger.LogAttrs(context.Background(), slog.LevelError, msg,
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
					slog.Duration("latency", v.Latency),
					slog.String("protocol", v.Protocol),
					slog.String("method", v.Method),
					slog.String("err", v.Error.Error()),
					slog.String("user_agent", v.UserAgent),
					slog.String("remote_ip", v.RemoteIP),
				)
			}
			return nil
		},
	}))

	JWT, err := jwtmiddleware.New()

	if err != nil {
		e.Logger.Fatalf("Unable to create JWT middleware: %v\n", err)
	}

	e.Use(JWT)
	e.Use(jwtmiddleware.UserID)

	db, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))

	if err != nil {
		e.Logger.Fatalf("Unable to connect to database: %v\n", err)
	}

	defer db.Close()

	e.GET("/list", func(c echo.Context) error {
		userID := c.Get("userID").(string)

		query, args := pg.SELECT(
			todo.Lists.ListID,
			todo.Lists.Title,
			todo.Lists.Description,
		).
			FROM(todo.Lists).
			WHERE(todo.Lists.UserID.EQ(pg.String(userID))).
			ORDER_BY(todo.Lists.ListID).
			Sql()

		rows, _ := db.Query(c.Request().Context(), query, args...)
		records, err := pgx.CollectRows(rows, pgx.RowToStructByName[ListsRecord])

		if err != nil {
			c.Logger().Errorf("Error fetching lists: %v\n", err)
			return InternalServerError
		}

		var lists = make([]ListResponse, 0, len(records))

		for _, record := range records {
			lists = append(lists, ListResponse(record))
		}

		return c.JSON(http.StatusOK, lists)
	})

	e.GET("/list/:list_id", func(c echo.Context) error {
		userID := c.Get("userID").(string)
		var listID int64

		if err := echo.PathParamsBinder(c).MustInt64("list_id", &listID).BindError(); err != nil {
			return err
		}

		query, args := pg.SELECT(
			todo.Lists.ListID,
			todo.Lists.Title,
			todo.Lists.Description,
		).
			FROM(todo.Lists).
			WHERE(
				todo.Lists.ListID.EQ(pg.Int(listID)).
					AND(todo.Lists.UserID.EQ(pg.String(userID))),
			).
			Sql()

		rows, _ := db.Query(c.Request().Context(), query, args...)
		record, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ListsRecord])

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return NotFoundError
			}
			c.Logger().Errorf("Error fetching list: %v\n", err)
			return InternalServerError
		}

		return c.JSON(http.StatusOK, ListResponse(record))
	})

	e.POST("/list", func(c echo.Context) error {
		userID := c.Get("userID").(string)
		var params ListRequest

		if err := c.Bind(&params); err != nil {
			return err
		}

		query, args := todo.Lists.INSERT(
			todo.Lists.Title,
			todo.Lists.Description,
			todo.Lists.UserID,
		).
			VALUES(
				params.Title,
				params.Description,
				userID,
			).
			RETURNING(
				todo.Lists.ListID,
				todo.Lists.Title,
				todo.Lists.Description,
			).
			Sql()

		rows, _ := db.Query(c.Request().Context(), query, args...)
		record, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ListsRecord])

		if err != nil {
			c.Logger().Errorf("Error creating list: %v\n", err)
			return InternalServerError
		}

		return c.JSON(http.StatusCreated, ListResponse(record))
	})

	e.PUT("/list/:list_id", func(c echo.Context) error {
		userID := c.Get("userID").(string)
		var listID int64

		if err := echo.PathParamsBinder(c).MustInt64("list_id", &listID).BindError(); err != nil {
			return err
		}

		var params ListRequest

		if err := c.Bind(&params); err != nil {
			return err
		}

		query, args := todo.Lists.
			UPDATE().
			SET(
				todo.Lists.Title.SET(pg.String(params.Title)),
				todo.Lists.Description.SET(pg.String(params.Description)),
			).
			WHERE(
				todo.Lists.ListID.EQ(pg.Int(listID)).
					AND(todo.Lists.UserID.EQ(pg.String(userID))),
			).
			RETURNING(
				todo.Lists.ListID,
				todo.Lists.Title,
				todo.Lists.Description,
			).
			Sql()

		rows, _ := db.Query(c.Request().Context(), query, args...)
		record, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ListsRecord])

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return NotFoundError
			}
			c.Logger().Errorf("Error updating list: %v\n", err)
			return InternalServerError
		}

		return c.JSON(http.StatusOK, ListResponse(record))
	})

	e.PATCH("/list/:list_id", func(c echo.Context) error {
		userID := c.Get("userID").(string)
		var listID int64

		if err := echo.PathParamsBinder(c).MustInt64("list_id", &listID).BindError(); err != nil {
			return err
		}

		var params ListPartialRequest

		if err := c.Bind(&params); err != nil {
			return err
		}

		stmt := todo.Lists.
			UPDATE().
			SET(todo.Lists.ListID.SET(pg.Int(listID))).
			WHERE(
				todo.Lists.ListID.EQ(pg.Int(listID)).
					AND(todo.Lists.UserID.EQ(pg.String(userID))),
			).
			RETURNING(
				todo.Lists.ListID,
				todo.Lists.Title,
				todo.Lists.Description,
			)

		if params.Title != nil {
			stmt = stmt.SET(todo.Lists.Title.SET(pg.String(*params.Title)))
		}

		if params.Description != nil {
			stmt = stmt.SET(todo.Lists.Description.SET(pg.String(*params.Description)))
		}

		query, args := stmt.Sql()

		rows, _ := db.Query(c.Request().Context(), query, args...)
		record, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ListsRecord])

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return NotFoundError
			}
			c.Logger().Errorf("Error updating list: %v\n", err)
			return InternalServerError
		}

		return c.JSON(http.StatusOK, ListResponse(record))
	})

	e.DELETE("/list/:list_id", func(c echo.Context) error {
		userID := c.Get("userID").(string)
		var listID int64

		if err := echo.PathParamsBinder(c).MustInt64("list_id", &listID).BindError(); err != nil {
			return err
		}

		query, args := todo.Lists.
			DELETE().
			WHERE(
				todo.Lists.ListID.EQ(pg.Int(listID)).
					AND(todo.Lists.UserID.EQ(pg.String(userID))),
			).
			Sql()

		_, err := db.Exec(c.Request().Context(), query, args...)

		if err != nil {
			c.Logger().Errorf("Error deleting list: %v\n", err)
			return InternalServerError
		}

		return c.NoContent(http.StatusNoContent)
	})

	e.GET("/list/:list_id/item", func(c echo.Context) error {
		userID := c.Get("userID").(string)
		var listID int64

		if err := echo.PathParamsBinder(c).MustInt64("list_id", &listID).BindError(); err != nil {
			return err
		}

		{
			query, args := pg.SELECT(pg.Int64(1)).
				FROM(todo.Lists).
				WHERE(
					todo.Lists.ListID.EQ(pg.Int(listID)).
						AND(todo.Lists.UserID.EQ(pg.String(userID))),
				).
				Sql()

			rows, _ := db.Query(c.Request().Context(), query, args...)
			_, err := pgx.CollectOneRow(rows, pgx.RowTo[int64])

			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return NotFoundError
				}
				c.Logger().Errorf("Error checking if list exists: %v\n", err)
				return InternalServerError
			}
		}

		query, args := pg.SELECT(
			todo.Items.ItemID,
			todo.Items.Content,
			todo.Items.IsComplete,
		).
			FROM(todo.Items).
			WHERE(todo.Items.ListID.EQ(pg.Int(listID))).
			ORDER_BY(todo.Items.ItemID).
			Sql()

		rows, _ := db.Query(c.Request().Context(), query, args...)
		records, err := pgx.CollectRows(rows, pgx.RowToStructByName[ItemsRecord])

		if err != nil {
			c.Logger().Errorf("Error fetching items: %v\n", err)
			return InternalServerError
		}

		var items = make([]ItemResponse, 0, len(records))

		for _, record := range records {
			items = append(items, ItemResponse(record))
		}

		return c.JSON(http.StatusOK, items)
	})

	e.GET("/list/:list_id/item/:item_id", func(c echo.Context) error {
		userID := c.Get("userID").(string)
		var listID, itemID int64

		if err := echo.PathParamsBinder(c).MustInt64("list_id", &listID).MustInt64("item_id", &itemID).BindError(); err != nil {
			return err
		}

		{
			query, args := pg.SELECT(pg.Int64(1)).
				FROM(todo.Lists).
				WHERE(
					todo.Lists.ListID.EQ(pg.Int(listID)).
						AND(todo.Lists.UserID.EQ(pg.String(userID))),
				).
				Sql()

			rows, _ := db.Query(c.Request().Context(), query, args...)
			_, err := pgx.CollectOneRow(rows, pgx.RowTo[int64])

			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return NotFoundError
				}
				c.Logger().Errorf("Error checking if list exists: %v\n", err)
				return InternalServerError
			}
		}

		query, args := pg.SELECT(
			todo.Items.ItemID,
			todo.Items.Content,
			todo.Items.IsComplete,
		).
			FROM(todo.Items).
			WHERE(
				todo.Items.ItemID.EQ(pg.Int(itemID)).
					AND(todo.Items.ListID.EQ(pg.Int(listID))),
			).
			Sql()

		rows, _ := db.Query(c.Request().Context(), query, args...)
		record, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ItemsRecord])

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return NotFoundError
			}
			c.Logger().Errorf("Error fetching item: %v\n", err)
			return InternalServerError
		}

		return c.JSON(http.StatusOK, ItemResponse(record))
	})

	e.POST("/list/:list_id/item", func(c echo.Context) error {
		userID := c.Get("userID").(string)
		var listID int64

		if err := echo.PathParamsBinder(c).MustInt64("list_id", &listID).BindError(); err != nil {
			return err
		}

		var params ItemRequest

		if err := c.Bind(&params); err != nil {
			return err
		}

		{
			query, args := pg.SELECT(pg.Int64(1)).
				FROM(todo.Lists).
				WHERE(
					todo.Lists.ListID.EQ(pg.Int(listID)).
						AND(todo.Lists.UserID.EQ(pg.String(userID))),
				).
				Sql()

			rows, _ := db.Query(c.Request().Context(), query, args...)
			_, err := pgx.CollectOneRow(rows, pgx.RowTo[int64])

			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return NotFoundError
				}
				c.Logger().Errorf("Error checking if list exists: %v\n", err)
				return InternalServerError
			}
		}

		query, args := todo.Items.
			INSERT(
				todo.Items.Content,
				todo.Items.IsComplete,
				todo.Items.ListID,
			).
			VALUES(
				params.Content,
				params.IsComplete,
				listID,
			).
			RETURNING(
				todo.Items.ItemID,
				todo.Items.Content,
				todo.Items.IsComplete,
			).
			Sql()

		rows, _ := db.Query(c.Request().Context(), query, args...)
		record, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ItemsRecord])

		if err != nil {
			c.Logger().Errorf("Error creating item: %v\n", err)
			return InternalServerError
		}

		return c.JSON(http.StatusCreated, ItemResponse(record))
	})

	e.PUT("/list/:list_id/item/:item_id", func(c echo.Context) error {
		userID := c.Get("userID").(string)
		var listID, itemID int64

		if err := echo.PathParamsBinder(c).MustInt64("list_id", &listID).MustInt64("item_id", &itemID).BindError(); err != nil {
			return err
		}

		var params ItemRequest

		if err := c.Bind(&params); err != nil {
			return err
		}

		{
			query, args := pg.SELECT(pg.Int64(1)).
				FROM(todo.Lists).
				WHERE(
					todo.Lists.ListID.EQ(pg.Int(listID)).
						AND(todo.Lists.UserID.EQ(pg.String(userID))),
				).
				Sql()

			rows, _ := db.Query(c.Request().Context(), query, args...)
			_, err := pgx.CollectOneRow(rows, pgx.RowTo[int64])

			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return NotFoundError
				}
				c.Logger().Errorf("Error checking if list exists: %v\n", err)
				return InternalServerError
			}
		}

		query, args := todo.Items.
			UPDATE().
			SET(
				todo.Items.Content.SET(pg.String(params.Content)),
				todo.Items.IsComplete.SET(pg.Bool(params.IsComplete)),
			).
			WHERE(
				todo.Items.ItemID.EQ(pg.Int(itemID)).
					AND(todo.Items.ListID.EQ(pg.Int(listID))),
			).
			RETURNING(
				todo.Items.ItemID,
				todo.Items.Content,
				todo.Items.IsComplete,
			).Sql()

		rows, _ := db.Query(c.Request().Context(), query, args...)
		record, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ItemsRecord])

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return NotFoundError
			}
			c.Logger().Errorf("Error updating item: %v\n", err)
			return InternalServerError
		}

		return c.JSON(http.StatusOK, ItemResponse(record))
	})

	e.PATCH("/list/:list_id/item/:item_id", func(c echo.Context) error {
		userID := c.Get("userID").(string)
		var listID, itemID int64

		if err := echo.PathParamsBinder(c).MustInt64("list_id", &listID).MustInt64("item_id", &itemID).BindError(); err != nil {
			return err
		}

		var params ItemPartialRequest

		if err := c.Bind(&params); err != nil {
			return err
		}

		{
			query, args := pg.SELECT(pg.Int64(1)).
				FROM(todo.Lists).
				WHERE(
					todo.Lists.ListID.EQ(pg.Int(listID)).
						AND(todo.Lists.UserID.EQ(pg.String(userID))),
				).
				Sql()

			rows, _ := db.Query(c.Request().Context(), query, args...)
			_, err := pgx.CollectOneRow(rows, pgx.RowTo[int64])

			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return NotFoundError
				}
				c.Logger().Errorf("Error checking if list exists: %v\n", err)
				return InternalServerError
			}
		}

		stmt := todo.Items.
			UPDATE().
			SET(todo.Items.ItemID.SET(pg.Int(itemID))).
			WHERE(
				todo.Items.ItemID.EQ(pg.Int(itemID)).
					AND(todo.Items.ListID.EQ(pg.Int(listID))),
			).
			RETURNING(
				todo.Items.ItemID,
				todo.Items.Content,
				todo.Items.IsComplete,
			)

		if params.Content != nil {
			stmt = stmt.SET(todo.Items.Content.SET(pg.String(*params.Content)))
		}

		if params.IsComplete != nil {
			stmt = stmt.SET(todo.Items.IsComplete.SET(pg.Bool(*params.IsComplete)))
		}

		query, args := stmt.Sql()

		rows, _ := db.Query(c.Request().Context(), query, args...)
		record, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ItemsRecord])

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return NotFoundError
			}
			c.Logger().Errorf("Error fetching item: %v\n", err)
			return InternalServerError
		}

		return c.JSON(http.StatusOK, ItemResponse(record))
	})

	e.DELETE("/list/:list_id/item/:item_id", func(c echo.Context) error {
		userID := c.Get("userID").(string)
		var listID, itemID int64

		if err := echo.PathParamsBinder(c).MustInt64("list_id", &listID).MustInt64("item_id", &itemID).BindError(); err != nil {
			return err
		}

		query, args := todo.Items.
			DELETE().
			USING(todo.Lists).
			WHERE(
				todo.Items.ListID.EQ(todo.Lists.ListID).
					AND(todo.Items.ItemID.EQ(pg.Int(itemID)).
						AND(todo.Items.ListID.EQ(pg.Int(listID)).
							AND(todo.Lists.UserID.EQ(pg.String(userID))))),
			).
			Sql()

		_, err := db.Exec(c.Request().Context(), query, args...)

		if err != nil {
			c.Logger().Errorf("Error deleting item: %v\n", err)
			return InternalServerError
		}

		return c.NoContent(http.StatusNoContent)
	})

	go func() {
		if err := e.Start(":8000"); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal(err)
		}
	}()

	<-ctx.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		e.Logger.Fatal(err)
	}
}
