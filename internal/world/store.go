package world

// SavesBackend is the storage capability a save store needs. It is
// satisfied by *storage.Saves; the indirection keeps this package free of
// database imports.
type SavesBackend interface {
	Load(playerID int64, worldKey string) ([]byte, error)
	Save(playerID int64, worldKey string, data []byte) error
}

// InventoryBackend is the storage capability an inventory API needs,
// satisfied by *storage.Inventory.
type InventoryBackend interface {
	Grant(playerID int64, worldKey, itemKey, name string, qty int) error
}

// boundSaveStore binds a backend to one (player, world) pair. The world
// only ever sees Load/Save with no parameters, so it physically cannot
// reach another slot.
type boundSaveStore struct {
	backend  SavesBackend
	playerID int64
	worldKey string
}

// NewSaveStore returns a SaveStore scoped to (playerID, worldKey).
func NewSaveStore(backend SavesBackend, playerID int64, worldKey string) SaveStore {
	return &boundSaveStore{backend: backend, playerID: playerID, worldKey: worldKey}
}

// Load implements SaveStore.
func (s *boundSaveStore) Load() ([]byte, error) {
	return s.backend.Load(s.playerID, s.worldKey)
}

// Save implements SaveStore.
func (s *boundSaveStore) Save(data []byte) error {
	return s.backend.Save(s.playerID, s.worldKey, data)
}

// boundInventory binds an inventory backend to one (player, world) pair.
type boundInventory struct {
	backend  InventoryBackend
	playerID int64
	worldKey string
}

// NewInventoryAPI returns an InventoryAPI scoped to (playerID, worldKey).
func NewInventoryAPI(backend InventoryBackend, playerID int64, worldKey string) InventoryAPI {
	return &boundInventory{backend: backend, playerID: playerID, worldKey: worldKey}
}

// Grant implements InventoryAPI.
func (i *boundInventory) Grant(itemKey, name string, qty int) error {
	return i.backend.Grant(i.playerID, i.worldKey, itemKey, name, qty)
}
