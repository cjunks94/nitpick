Rails.application.routes.draw do
  mount Rswag::Ui::Engine => "/api-docs"
  mount Rswag::Api::Engine => "/api-docs"
  # Define your application routes per the DSL in https://guides.rubyonrails.org/routing.html

  # Reveal health status on /up that returns 200 if the app boots with no exceptions, otherwise 500.
  # Can be used by load balancers and uptime monitors to verify that the app is live.
  get "up" => "rails/health#show", as: :rails_health_check
  get "health" => "health#show"

  # Render dynamic PWA files from app/views/pwa/*
  get "service-worker" => "rails/pwa#service_worker", as: :pwa_service_worker
  get "manifest" => "rails/pwa#manifest", as: :pwa_manifest

  namespace :api do
    namespace :v1 do
      resources :connections, only: [ :index, :show ] do
        post :probe, on: :member
      end
      resources :pipelines, only: [ :index, :show ] do
        post :run, on: :member
        resources :runs, only: [ :index ], controller: "runs"
      end
      resources :mappings, only: [ :index, :show ]
      resources :widgets, only: [ :index, :show, :create, :update, :destroy ]
      resources :runs, only: [ :show ]
    end
  end

  root "dashboard#show"
  get "pipelines/:id", to: "pipelines#show", as: :pipeline
  get "pipelines/:id/edit", to: "pipelines#edit", as: :edit_pipeline
  patch "pipelines/:id", to: "pipelines#update"
  get "pipelines/:id/versions", to: "config_versions#index", as: :pipeline_versions,
      defaults: { configurable_type: "Pipeline" }
  post "pipelines/:pipeline_id/run", to: "pipeline_runs#create", as: :run_pipeline

  post "connections/:id/probe", to: "connections#probe", as: :probe_connection

  get "mappings/:id/edit", to: "mappings#edit", as: :edit_mapping
  patch "mappings/:id", to: "mappings#update", as: :mapping
  get "mappings/:id/versions", to: "config_versions#index", as: :mapping_versions,
      defaults: { configurable_type: "Mapping" }

  get "runs/:id", to: "pipeline_runs#show", as: :pipeline_run
  get "export_runs/:id/download", to: "export_runs#download", as: :download_export_run
end
