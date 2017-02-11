
struct DriverLocation {
  1: required string  driver_id
  2: required string  location
}

service Driver  {
    list<DriverLocation> findNearest(1: string location)
}