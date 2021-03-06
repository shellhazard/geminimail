## geminimail

A Gemini service for generating disposable email addresses, powered by 10minutemail.

![geminimail accessed through the Lagrange browser](screenshot.png)

### Usage

Generate a self-signed certificate for the server to use. If you're unsure, you can use the example utility provided by the `go-gemini` library, where `localhost` is the host you want to sign for:
```
git clone git.sr.ht/~adnano/go-gemini
cd ./go-gemini/examples
go run cert.go localhost 8760h
```

This will create a `localhost.crt` and `localhost.key` pair, valid for 365 days. Clone this repository and copy those files to the root. Start the server with:
```
go run main.go localhost
```

And visit `gemini://localhost` in your preferred Gemini client to see it in action.