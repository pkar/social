package facebook

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"

	"code.google.com/p/goauth2/oauth"
	log "github.com/golang/glog"
)

// RequestTypes define the facebook api queries for profiles.
// Make sure to jslint any changes here.
var requestTypes = map[string]string{
	"full": `
[
  {"method": "GET", "relative_url": "me?fields=picture.type(large),id,birthday,email,gender,last_name,first_name,link,locale,location,name,timezone,username"},
  {"method": "GET", "relative_url": "me/likes?limit=10", "name": "get-likes"},
  {"method": "GET", "relative_url": "me/friends?limit=500", "name": "get-friends"},
  {"method": "GET", "relative_url": "?ids={result=get-friends:$.data.*.id}&fields=picture.type(large),id,birthday,email,gender,last_name,first_name,link,locale,location,name,timezone,username"},
  {"method": "GET", "relative_url": "?ids={result=get-likes:$.data.*.id}&fields=id,picture.type(large),category,link,name,username,website,location"},
  {"method": "GET", "relative_url": "me/feed?limit=10"},
  {"method": "GET", "relative_url": "me/home?limit=10"},
  {"method": "GET", "relative_url": "me/photos?limit=10"},
  {"method": "GET", "relative_url": "me/videos?limit=10"}
]
`,
	"lite": `
[
  {"method": "GET", "relative_url": "me?fields=picture.type(large),id,birthday,email,gender,last_name,first_name,link,locale,location,name,timezone,username"},
  {"method": "GET", "relative_url": "me/likes?limit=1", "name": "get-likes"},
  {"method": "GET", "relative_url": "me/friends?limit=200", "name": "get-friends"},
  {"method": "GET", "relative_url": "?ids={result=get-friends:$.data.*.id}&fields=picture.type(large),id,birthday,email,gender,last_name,first_name,link,locale,location,name,timezone,username"},
  {"method": "GET", "relative_url": "?ids={result=get-likes:$.data.*.id}&fields=id,picture.type(large),category,link,name,username,website,location"},
  {"method": "GET", "relative_url": "me/feed?limit=1"},
  {"method": "GET", "relative_url": "me/home?limit=1"},
  {"method": "GET", "relative_url": "me/photos?limit=1"},
  {"method": "GET", "relative_url": "me/videos?limit=1"}
]
`,
}

// Conf for requests.
type Conf struct {
	ClientID     string `json:"ClientID"`
	ClientSecret string `json:"ClientSecret"`
	AuthURL      string `json:"AuthURL"`
	TokenURL     string `json:"TokenURL"`
	Redirect     string `json:"Redirect"`
	Scope        string `json:"Scope"`
}

// Facebook is the service interface implementation.
type Facebook struct {
	Config *oauth.Config
}

// Profile meta for a user.
type Profile struct {
	ID      string   `json:"ID" bson:"_id"`
	Token   string   `json:"Token" bson:"Token"`
	Me      *Me      `json:"Me" bson:"Me"`
	Feed    *Feed    `json:"Feed" bson:"Feed"`
	Friends *Friends `json:"Friends" bson:"Friends"`
	Likes   *Likes   `json:"Likes" bson:"Likes"`
	Home    *Home    `json:"Home" bson:"Home"`
	Photos  *Photos  `json:"Photos" bson:"Photos"`
	Videos  *Videos  `json:"Videos" bson:"Videos"`
}

// NewConfig for the oauth login flow. Settings defined in
// $GOPATH/config/conf.json and loaded in $GOPATH/src/config/config.go
func NewConfig(conf *Conf) *oauth.Config {
	return &oauth.Config{
		ClientId:     conf.ClientID,
		ClientSecret: conf.ClientSecret,
		RedirectURL:  conf.Redirect,
		AuthURL:      conf.AuthURL,
		TokenURL:     conf.TokenURL,
		Scope:        conf.Scope,
	}
}

// New creates a facebook object that can do requests.
func New(conf *Conf) (*Facebook, error) {
	fb := &Facebook{
		Config: NewConfig(conf),
	}
	return fb, nil
}

// facebookRequest makes a remote call to the graph api to pull in
// a users profile.
var facebookRequest = func(transport *oauth.Transport, queryType string) ([]byte, error) {
	var query string
	var ok bool
	if query, ok = requestTypes[queryType]; !ok {
		return nil, fmt.Errorf("facebook query definition not found: " + queryType)
	}
	values := url.Values{}
	values.Set("access_token", transport.Token.AccessToken)
	values.Set("batch", query)
	resp, err := transport.Client().PostForm("https://graph.facebook.com", values)
	if err != nil {
		log.Error("PostForm:", err)
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return body, nil
}

// Request makes a remote call to the to pull in
// a users profile.
func (f *Facebook) Request(token, queryType string) (*Profile, error) {
	if token == "" {
		return nil, fmt.Errorf("token is empty")
	}
	transport := &oauth.Transport{Config: f.Config}
	transport.Token = &oauth.Token{AccessToken: token}
	resp, err := facebookRequest(transport, queryType)
	if err != nil {
		return nil, err
	}
	profile, err := f.LoadData(resp)
	if err != nil {
		return nil, err
	}
	return profile, nil
}

// LoadData processes a facebook batch request into
// an array of the requests.
// Basically golang json into an array of mess that is
// the facebook api is not easy.
// Some values returned are nil which breaks json decoding
func (f *Facebook) LoadData(jsonResp []byte) (*Profile, error) {
	var payload []map[string]interface{}
	err := json.Unmarshal(jsonResp, &payload)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	fbProfile := &Profile{}
	for i, d := range payload {
		code, ok := d["code"].(float64)
		if !ok || int(code) != 200 {
			obj := ""
			switch i {
			case 0:
				obj = "me"
			case 1:
				//obj = "get-likes"
			case 2:
				//obj = "get-friends"
			case 3:
				obj = "friends"
			case 4:
				obj = "like"
			case 5:
				obj = "feed"
			case 6:
				obj = "home"
			case 7:
				obj = "photos"
			case 8:
				obj = "vidoes"
			default:
			}
			if obj != "" {
				log.Errorf("response code not 200 %s %#v", obj, d)
			}
			continue
		}
		d, ok := d["body"].(string)
		if !ok {
			log.Error("body missing ", d)
			continue
		}
		data := []byte(d)

		switch i {
		// Me
		case 0:
			var me Me
			err = json.Unmarshal(data, &me)
			if err != nil {
				log.Error(err)
				continue
			}
			fbProfile.Me = &me
		// Friends
		case 3:
			var friends Friends
			err = json.Unmarshal(data, &friends)
			if err != nil {
				log.Error(err, data)
				continue
			}
			fbProfile.Friends = &friends
		// Likes
		case 4:
			var likes Likes
			err = json.Unmarshal(data, &likes)
			if err != nil {
				log.Error(err)
				continue
			}
			fbProfile.Likes = &likes
		// Feed
		case 5:
			var feed Feed
			err = json.Unmarshal(data, &feed)
			if err != nil {
				log.Error(err)
				continue
			}
			fbProfile.Feed = &feed
		// Home
		case 6:
			var home Home
			err = json.Unmarshal(data, &home)
			if err != nil {
				log.Error(err)
				continue
			}
			fbProfile.Home = &home
		// Photos
		case 7:
			var photos Photos
			err = json.Unmarshal(data, &photos)
			if err != nil {
				log.Error(err)
				continue
			}
			fbProfile.Photos = &photos
		// Photos
		case 8:
			var videos Videos
			err = json.Unmarshal(data, &videos)
			if err != nil {
				log.Error(err)
				continue
			}
			fbProfile.Videos = &videos
		}
	}
	return fbProfile, nil
}

// Me ...
type Me struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	UserName  string `json:"username"`
	Birthday  string `json:"birthday"`
	Education []struct {
		School struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"school"`
		Type string `json:"type"`
	} `json:"education"`
	Gender   string `json:"gender"`
	Hometown struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"hometown"`
	Picture struct {
		Data struct {
			URL         string `json:"url"`
			IsSilhoutte bool   `json:"is_silhoutte"`
		} `json:"data"`
	} `json:"picture"`
	Link     string `json:"link"`
	Locale   string `json:"locale"`
	Location struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"location"`
	Name        string `json:"name"`
	Timezone    int64  `json:"timezone"`
	UpdatedTime string `json:"updated_time"`
	Verified    bool   `json:"verified"`
	Work        []struct {
		Employer struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"employer"`
	} `json:"work"`
}

// Friends ...
type Friends map[string]struct {
	ID        string `json:"id"`
	Bio       string `json:"bio"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	UserName  string `json:"username"`
	Email     string `json:"email"`
	Gender    string `json:"gender"`
	Picture   struct {
		Data struct {
			URL         string `json:"url"`
			IsSilhoutte bool   `json:"is_silhoutte"`
		} `json:"data"`
	} `json:"picture"`
	Link        string `json:"link"`
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Birthday    string `json:"birthday"`
	UpdatedTime string `json:"updated_time"`
	Timezone    int64  `json:"timezone"`
}

// Feed ...
type Feed struct {
	Data []struct {
		Application struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"application"`
		Actions []struct {
			Link string `json:"link"`
			Name string `json:"name"`
		} `json:"actions"`
		Caption     string `json:"caption"`
		CreatedTime string `json:"created_time"`
		Description string `json:"description"`
		From        struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"from"`
		Icon    string `json:"icon"`
		ID      string `json:"id"`
		Privacy struct {
			Deny        string `json:"deny"`
			Value       string `json:"value"`
			Description string `json:"description"`
			Allow       string `json:"allow"`
			Friends     string `json:"friends"`
			Network     string `json:"network"`
		} `json:"privacy"`
		Link      string `json:"link"`
		Story     string `json:"story"`
		StoryTags map[string][]struct {
			ID     string `json:"id"`
			Length int64  `json:"length"`
			Name   string `json:"name"`
			Offset int64  `json:"offset"`
			Type   string `json:"type"`
		} `json:"story_tags"`
		Type        string `json:"type"`
		UpdatedTime string `json:"updated_time"`
		StatusType  string `json:"status_type"`
		Picture     string `json:"picture"`
	} `json:"data"`
	Paging struct {
		Next     string `json:"next"`
		Previous string `json:"previous"`
	} `json:"paging"`
}

// LikesLocation ...
type LikesLocation struct {
	City      string  `json:"city"`
	Country   string  `json:"country"`
	State     string  `json:"state"`
	Street    string  `json:"street"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Zip       string  `json:"zip"`
}

// Likes ...
type Likes map[string]struct {
	About    string `json:"about"`
	CanPost  bool   `json:"can_post"`
	Category string `json:"category"`
	Checkins int64  `json:"checkins,omitempty"`
	Cover    struct {
		CoverID interface{} `json:"cover_id"` // facebook fucked data, sometimes string sometimes int
		OffsetX int64       `json:"offset_x"`
		OffsetY int64       `json:"offset_y"`
		Source  string      `json:"source"`
	} `json:"cover"`
	Picture struct {
		Data struct {
			URL         string `json:"url"`
			IsSilhoutte bool   `json:"is_silhoutte"`
		} `json:"data"`
	} `json:"picture"`

	ID                string      `json:"id"`
	IsPublished       bool        `json:"is_published"`
	Likes             interface{} `json:"likes"` // facebook fucked data
	Link              string      `json:"link"`
	Name              string      `json:"name"`
	TalkingAboutCount int64       `json:"talking_about_count"`
	Username          string      `json:"username"`
	Website           string      `json:"website"`
	WereHereCount     int64       `json:"were_here_count"`

	// TODO adding json tag causes linker "name too long error" in 1.2
	//Location *LikesLocation //`json:"location,omitempty"`
	//	Location struct {
	//		City      string  `json:"city"`
	//		Country   string  `json:"country"`
	//		State     string  `json:"state"`
	//		Street    string  `json:"street"`
	//		Latitude  float64 `json:"latitude"`
	//		Longitude float64 `json:"longitude"`
	//		Zip       string  `json:"zip"`
	//	} `json:"location,omitempty"`

	Mission         string `json:"mission,omitempty"`
	CompanyOverview string `json:"company_overview,omitempty"`
	IsCommunityPage bool   `json:"is_community_page,omitempty"`
	Founded         string `json:"founded,omitempty"`
	Awards          string `json:"awards,omitempty"`
	Network         string `json:"network,omitempty"`
	PlotOutline     string `json:"plot_outline,omitempty"`
	ReleaseDate     string `json:"release_date,omitempty"`
	Description     string `json:"description,omitempty"`
	DirectedBy      string `json:"directed_by,omitempty"`
	Genre           string `json:"genre,omitempty"`
	Schedule        string `json:"schedule,omitempty"`
	Season          string `json:"season,omitempty"`
	Starring        string `json:"starring,omitempty"`
	WrittenBy       string `json:"written_by,omitempty"`
}

// Home ...
type Home struct {
	Data []struct {
		Actions []struct {
			Link string `json:"link"`
			Name string `json:"name"`
		} `json:"actions"`
		Application struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"application"`
		Caption     string `json:"caption"`
		CreatedTime string `json:"created_time"`
		Description string `json:"description"`
		From        struct {
			Category string `json:"category"`
			ID       string `json:"id"`
			Name     string `json:"name"`
		} `json:"from"`
		Icon  string `json:"icon"`
		ID    string `json:"id"`
		Likes struct {
			Data []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"data"`
			Paging struct {
				Cursors struct {
					After  string `json:"after"`
					Before string `json:"before"`
				} `json:"cursors"`
			} `json:"paging"`
		} `json:"likes"`
		Link    string `json:"link"`
		Message string `json:"message"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
		Privacy struct {
			Value string `json:"value"`
		} `json:"privacy"`
		StatusType  string `json:"status_type"`
		Type        string `json:"type"`
		UpdatedTime string `json:"updated_time"`
	} `json:"data"`
	Paging struct {
		Next     string `json:"next"`
		Previous string `json:"previous"`
	} `json:"paging"`
}

// Photos ...
type Photos struct {
	Data []struct {
		CreatedTime string `json:"created_time"`
		UpdatedTime string `json:"updated_time"`
		From        struct {
			Category string `json:"category"`
			ID       string `json:"id"`
			Name     string `json:"name"`
		} `json:"from"`
		Height uint   `json:"height"`
		Width  uint   `json:"width"`
		Icon   string `json:"icon"`
		ID     string `json:"id"`
		Images []struct {
			Height uint   `json:"height"`
			Source string `json:"source"`
			Width  uint   `json:"width"`
		} `json:"images"`
		Likes struct {
			Data []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"data"`
			Paging struct {
				Cursors struct {
					After  string `json:"after"`
					Before string `json:"before"`
				} `json:"cursors"`
			} `json:"paging"`
		} `json:"likes"`
		Link    string `json:"link"`
		Picture string `json:"picture"`
		Source  string `json:"source"`
		Place   struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Location struct {
				City      string  `json:"city"`
				Country   string  `json:"country"`
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
				State     string  `json:"state"`
				Street    string  `json:"street"`
				Zip       string  `json:"zip"`
			} `json:"location"`
		} `json:"place"`
		Tags struct {
			Data []struct {
				CreatedTime string  `json:"created_time"`
				ID          string  `json:"id"`
				Name        string  `json:"name"`
				X           float64 `json:"x"`
				Y           float64 `json:"y"`
			} `json:"data"`
			Paging struct {
				Next     string `json:"next"`
				Previous string `json:"previous"`
			} `json:"paging"`
		} `json:"tags"`
	} `json:"data"`
}

// Videos ...
type Videos struct {
	Data []struct {
		CreatedTime string `json:"created_time"`
		UpdatedTime string `json:"updated_time"`
		From        struct {
			Category string `json:"category"`
			ID       string `json:"id"`
			Name     string `json:"name"`
		} `json:"from"`
		ID     string `json:"id"`
		Images []struct {
			Height uint   `json:"height"`
			Source string `json:"source"`
			Width  uint   `json:"width"`
		} `json:"images"`
		Likes struct {
			Data []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"data"`
			Paging struct {
				Cursors struct {
					After  string `json:"after"`
					Before string `json:"before"`
				} `json:"cursors"`
			} `json:"paging"`
		} `json:"likes"`
		Picture string `json:"picture"`
		Source  string `json:"source"`
		Tags    struct {
			Data []struct {
				CreatedTime string  `json:"created_time"`
				ID          string  `json:"id"`
				Name        string  `json:"name"`
				X           float64 `json:"x"`
				Y           float64 `json:"y"`
			} `json:"data"`
			Paging struct {
				Next     string `json:"next"`
				Previous string `json:"previous"`
			} `json:"paging"`
		} `json:"tags"`
	} `json:"data"`
}
