package repository

import "github.com/jackc/pgx/v5/pgxpool"

// Repos aggregates all repository interfaces for dependency injection.
type Repos struct {
	Users   UserRepository
	Sites   SiteRepository
	Events  EventRepository
	Stats   StatsRepository
	Admin   AdminRepository
	CMS     CMSRepository
	Goals   GoalRepository
	Funnels FunnelRepository
}

// New creates a Repos with all pg implementations wired up.
func New(pool *pgxpool.Pool) *Repos {
	return &Repos{
		Users:   &pgUserRepository{pool: pool},
		Sites:   &pgSiteRepository{pool: pool},
		Events:  &pgEventRepository{pool: pool},
		Stats:   &pgStatsRepository{pool: pool},
		Admin:   &pgAdminRepository{pool: pool},
		CMS:     &pgCMSRepository{pool: pool},
		Goals:   &pgGoalRepository{pool: pool},
		Funnels: &pgFunnelRepository{pool: pool},
	}
}
