package vertesia_test

import (
	"context"

	vertesia "github.com/vertesia/vertesia-client-go"
)

func ExampleNewClient_apiKey() {
	client, err := vertesia.NewClient(vertesia.ClientOptions{
		APIKey: "sk-...",
	})
	if err != nil {
		panic(err)
	}

	account, _, err := client.AccountsAPI.GetCurrentAccount(context.Background()).Execute()
	if err != nil {
		panic(err)
	}

	_ = account
}

func ExampleNewClient_token() {
	client, err := vertesia.NewClient(vertesia.ClientOptions{
		Token: "eyJ...",
	})
	if err != nil {
		panic(err)
	}

	objects, _, err := client.ObjectsAPI.SearchObjects(context.Background()).Execute()
	if err != nil {
		panic(err)
	}

	_ = objects
}
