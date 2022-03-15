package mixpanel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

var IgnoreTime *time.Time = &time.Time{}

type MixpanelError struct {
	URL string
	Err error
}

func (err *MixpanelError) Cause() error {
	return err.Err
}

func (err *MixpanelError) Error() string {
	return "mixpanel: " + err.Err.Error()
}

type ErrTrackFailed struct {
	Body string
	Resp *http.Response
}

func (err *ErrTrackFailed) Error() string {
	return fmt.Sprintf("Mixpanel did not return 1 when tracking: %s", err.Body)
}

// The Mixapanel struct store the mixpanel endpoint and the project token
type Mixpanel interface {
	// Create a mixpanel event
	Track(distinctId, eventName string, e *Event) error

	// Set properties for a mixpanel user.
	UpdateUser(distinctId string, u *Update) error

	// Set properties for a union on user
	UnionUser(userID string, u *Update) error

	// Set properties for a mixpanel group.
	UpdateGroup(groupKey, groupId string, u *Update) error

	// Set properties for a union on group
	UnionGroup(groupKey, groupId string, u *Update) error

	Alias(distinctId, newId string) error
}

// The Mixapanel struct store the mixpanel endpoint and the project token
type mixpanel struct {
	Client *http.Client
	Token  string
	ApiURL string
}

// A mixpanel event
type Event struct {
	// IP-address of the user. Leave empty to use autodetect, or set to "0" to
	// not specify an ip-address.
	IP string

	// Timestamp. Set to nil to use the current time.
	Timestamp *time.Time

	// Custom properties. At least one must be specified.
	Properties map[string]interface{}
}

// An update of a user in mixpanel
type Update struct {
	// IP-address of the user. Leave empty to use autodetect, or set to "0" to
	// not specify an ip-address at all.
	IP string

	// Timestamp. Set to nil to use the current time, or IgnoreTime to not use a
	// timestamp.
	Timestamp *time.Time

	// Update operation such as "$set", "$update" etc.
	Operation string

	// Custom properties. At least one must be specified.
	Properties map[string]interface{}
}

// Track create a events to current distinct id
func (m *mixpanel) Alias(distinctId, newId string) error {
	props := map[string]interface{}{
		"token":       m.Token,
		"distinct_id": distinctId,
		"alias":       newId,
	}

	params := map[string]interface{}{
		"event":      "$create_alias",
		"properties": props,
	}

	return m.sendPost("track", params)
}

// Track create a events to current distinct id
func (m *mixpanel) Track(distinctId, eventName string, e *Event) error {
	props := map[string]interface{}{
		"token":       m.Token,
		"distinct_id": distinctId,
	}
	if e.IP != "" {
		props["ip"] = e.IP
	}
	if e.Timestamp != nil {
		props["time"] = e.Timestamp.Unix()
	}

	for key, value := range e.Properties {
		props[key] = value
	}

	params := map[string]interface{}{
		"event":      eventName,
		"properties": props,
	}

	return m.sendPost("track", params)
}

// UpdateUser: Updates a user in mixpanel. See
// https://mixpanel.com/help/reference/http#people-analytics-updates
func (m *mixpanel) UpdateUser(distinctId string, u *Update) error {
	params := map[string]interface{}{
		"$token":       m.Token,
		"$distinct_id": distinctId,
	}

	if u.IP != "" {
		params["$ip"] = u.IP
	}
	if u.Timestamp == IgnoreTime {
		params["$ignore_time"] = true
	} else if u.Timestamp != nil {
		params["$time"] = u.Timestamp.Unix()
	}

	params[u.Operation] = u.Properties

	return m.sendPost("engage", params)
}

// UnionGroup: Unions a group property in mixpanel. See
// https://api.mixpanel.com/engage#profile-union
func (m *mixpanel) UnionUser(userID string, u *Update) error {
	params := map[string]interface{}{
		"$token":       m.Token,
		"$distinct_id": userID,
	}

	params[u.Operation] = u.Properties

	return m.sendPost("engage#profile-union", params)
}

// UpdateUser: Updates a group in mixpanel. See
// https://api.mixpanel.com/groups#group-set
func (m *mixpanel) UpdateGroup(groupKey, groupId string, u *Update) error {
	params := map[string]interface{}{
		"$token":     m.Token,
		"$group_id":  groupId,
		"$group_key": groupKey,
	}

	params[u.Operation] = u.Properties

	return m.sendPost("groups", params)
}

// UnionGroup: Unions a group property in mixpanel. See
// https://api.mixpanel.com/groups#group-union
func (m *mixpanel) UnionGroup(groupKey, groupId string, u *Update) error {
	params := map[string]interface{}{
		"$token":     m.Token,
		"$group_id":  groupId,
		"$group_key": groupKey,
	}

	params[u.Operation] = u.Properties

	return m.sendPost("groups#group-union", params)
}

func (m *mixpanel) sendPost(eventType string, params interface{}) error {
	// params needs to be an array
	params = []interface{}{params}
	url := m.ApiURL + "/" + eventType

	wrapErr := func(err error) error {
		return &MixpanelError{URL: url, Err: err}
	}

	postBody, err := json.Marshal(params)
	if err != nil {
		wrapErr(&ErrTrackFailed{Body: err.Error(), Resp: nil})
	}

	responseBody := bytes.NewBuffer(postBody)

	//Leverage Go's HTTP Post function to make request
	resp, err := http.Post(url, "application/json", responseBody)
	//Handle Error
	if err != nil {
		wrapErr(&ErrTrackFailed{Body: err.Error(), Resp: resp})
	}
	defer resp.Body.Close()
	//Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		wrapErr(&ErrTrackFailed{Body: err.Error(), Resp: resp})
	}
	sb := string(body)
	if sb != "1" {
		return wrapErr(&ErrTrackFailed{Body: "response not 1", Resp: resp})
	}

	return nil
}

// New returns the client instance. If apiURL is blank, the default will be used
// ("https://api.mixpanel.com").
func New(token, apiURL string) Mixpanel {
	return NewFromClient(http.DefaultClient, token, apiURL)
}

// Creates a client instance using the specified client instance. This is useful
// when using a proxy.
func NewFromClient(c *http.Client, token, apiURL string) Mixpanel {
	if apiURL == "" {
		apiURL = "https://api.mixpanel.com"
	}

	return &mixpanel{
		Client: c,
		Token:  token,
		ApiURL: apiURL,
	}
}
