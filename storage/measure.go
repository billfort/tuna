package storage

import (
	"fmt"
	"github.com/nknorg/tuna/util"
	"log"
	"math"
	"net"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxFavoriteLength = 10
	maskSize          = 16
	favoriteExpired   = 14 * 24 * time.Hour
	avoidExpired      = 7 * 24 * time.Hour
	avoidCIDRMinIP    = 3

	FavoriteFileName = "favorite-node.json"
	AvoidFileName    = "avoid-node.json"
)

type FavoriteNode struct {
	IP           string  `json:"ip"`
	Address      string  `json:"address"`
	Metadata     string  `json:"metadata"`
	Delay        float32 `json:"delay"`
	MinBandwidth float32 `json:"minBandwidth"`
	MaxBandwidth float32 `json:"maxBandwidth"`
	ExpiresAt    int64   `json:"expiredAt"`
}

type AvoidNodes = map[string]*AvoidNode

type AvoidNode struct {
	IP        string `json:"ip"`
	MaskSize  int32  `json:"maskSize"`
	Address   string `json:"address"`
	ExpiresAt int64  `json:"expiredAt"`
}

type MeasureStorage struct {
	path             string
	favoriteFilePath string
	avoidFilePath    string

	FavoriteNodes *Storage

	avoidNodeMutex sync.RWMutex
	AvoidNodes     map[string]AvoidNodes

	avoidNodeFileMutex    sync.RWMutex
	favoriteNodeFileMutex sync.RWMutex
}

func NewMeasureStorage(path string) *MeasureStorage {
	return &MeasureStorage{
		path:             path,
		favoriteFilePath: filepath.Join(path, FavoriteFileName),
		avoidFilePath:    filepath.Join(path, AvoidFileName),
	}
}

func (s *MeasureStorage) Load() error {
	isExists := util.Exists(s.favoriteFilePath)
	if !isExists {
		err := util.WriteJSON(s.favoriteFilePath, map[string]*FavoriteNode{})
		if err != nil {
			return err
		}
	}
	var favoriteData map[string]*FavoriteNode
	err := util.ReadJSON(s.favoriteFilePath, &favoriteData)
	if err != nil {
		err := util.WriteJSON(s.favoriteFilePath, map[string]*FavoriteNode{})
		if err != nil {
			return err
		}
	}

	s.FavoriteNodes = NewStorage()
	for k, v := range favoriteData {
		s.FavoriteNodes.Add(k, v)
	}

	isExists = util.Exists(s.avoidFilePath)
	if !isExists {
		err := util.WriteJSON(s.avoidFilePath, map[string]*AvoidNode{})
		if err != nil {
			return err
		}
	}
	var avoidData map[string]AvoidNodes
	err = util.ReadJSON(s.avoidFilePath, &avoidData)
	if err != nil {
		err := util.WriteJSON(s.avoidFilePath, map[string]*AvoidNode{})
		if err != nil {
			return err
		}
	}
	s.AvoidNodes = avoidData

	err = s.ClearFavoriteExpired()
	if err != nil {
		return err
	}
	err = s.ClearAvoidExpired()
	if err != nil {
		return err
	}
	return nil
}

func (s *MeasureStorage) ClearFavoriteExpired() error {
	for k, v := range s.FavoriteNodes.GetData() {
		if time.Now().Unix() > v.(*FavoriteNode).ExpiresAt {
			s.FavoriteNodes.Delete(k)
		}
	}
	err := s.SaveFavoriteNodes()
	return err
}

func (s *MeasureStorage) ClearAvoidExpired() error {
	s.avoidNodeMutex.Lock()
	defer s.avoidNodeMutex.Unlock()
	for k1, v1 := range s.AvoidNodes {
		for k2, v2 := range v1 {
			if time.Now().Unix() > v2.ExpiresAt {
				delete(v1, k2)
			}
		}
		if len(v1) == 0 {
			delete(s.AvoidNodes, k1)
		}
	}
	err := s.SaveAvoidNodes()

	return err
}

func (s *MeasureStorage) SaveFavoriteNodes() error {
	s.favoriteNodeFileMutex.Lock()
	defer s.favoriteNodeFileMutex.Unlock()
	err := util.WriteJSON(s.favoriteFilePath, s.FavoriteNodes.GetData())
	if err != nil {
		return err
	}
	return nil
}

func (s *MeasureStorage) SaveAvoidNodes() error {
	s.avoidNodeFileMutex.Lock()
	defer s.avoidNodeFileMutex.Unlock()
	err := util.WriteJSON(s.avoidFilePath, s.AvoidNodes)
	if err != nil {
		return err
	}
	return nil
}

func (s *MeasureStorage) AddFavoriteNode(key string, val *FavoriteNode) bool {
	if val.ExpiresAt == 0 {
		val.ExpiresAt = time.Now().Add(favoriteExpired).Unix()
	}

	if s.FavoriteNodes.Len() >= maxFavoriteLength {
		minBandwidth := float32(0)
		for _, v := range s.FavoriteNodes.GetData() {
			item := v.(*FavoriteNode)
			if item.MinBandwidth < minBandwidth || minBandwidth == 0 {
				minBandwidth = item.MinBandwidth
			}
		}
		if val.MinBandwidth > minBandwidth {
			s.FavoriteNodes.Add(key, val)
			deleteKey := ""
			minExpire := int64(math.MaxInt32)
			for k, v := range s.FavoriteNodes.GetData() {
				item := v.(*FavoriteNode)
				if item.ExpiresAt < minExpire {
					minExpire = item.ExpiresAt
					deleteKey = k
				}
			}
			s.FavoriteNodes.Delete(deleteKey)
			return true
		}

	} else {
		s.FavoriteNodes.Add(key, val)
		return true
	}
	return false
}

func (s *MeasureStorage) AddAvoidNode(key string, val *AvoidNode) {
	s.avoidNodeMutex.Lock()
	defer s.avoidNodeMutex.Unlock()
	if val.ExpiresAt == 0 {
		val.ExpiresAt = time.Now().Add(avoidExpired).Unix()
	}

	if val.MaskSize == 0 {
		val.MaskSize = maskSize
	}

	_, subnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", key, val.MaskSize))
	if err != nil {
		log.Println(err)
		return
	}

	if _, ok := s.AvoidNodes[subnet.String()]; ok {
		s.AvoidNodes[subnet.String()][key] = val
	} else {
		s.AvoidNodes[subnet.String()] = map[string]*AvoidNode{
			val.IP: val,
		}
	}
}

func (s *MeasureStorage) GetAvoidCIDR() []*net.IPNet {
	s.avoidNodeMutex.RLock()
	defer s.avoidNodeMutex.RUnlock()
	var results []*net.IPNet
	for k, v := range s.AvoidNodes {
		if len(v) > avoidCIDRMinIP {
			_, subnet, err := net.ParseCIDR(k)
			if err != nil {
				log.Printf("parseCIDR error: %s", k)
				continue
			}
			results = append(results, subnet)
		}
	}
	return results
}