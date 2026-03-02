package agent

func (m *AutonomousRunManager) Reset() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stateStore != nil {
		if err := m.stateStore.Save(nil); err != nil {
			return err
		}
	}
	m.runs = make(map[string]*autonomousRunState)
	m.order = nil
	m.seq = 0
	return nil
}
